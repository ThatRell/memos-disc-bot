package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/bwmarrin/discordgo"
)

const (
	memosURL = "http://localhost:5230"
)

func transferServer(s *discordgo.Session, g *discordgo.Guild) {
	categoryToChannels, categoryObjects, uncategorizedChannels, activeThreadMap, err := parseServer(s, g)
	if err != nil {
		fmt.Println("Error parsing discord server:", err)
		return
	}

	spaceReq := SpaceReq{
		Title: g.Name,
	}

	jsonData, err := json.Marshal(spaceReq)
	if err != nil {
		fmt.Println("Failed to marshal JSON:", err)
		return
	}

	bodyReader := bytes.NewBuffer(jsonData)

	endpoint := fmt.Sprintf("%s/api/v1/spaces", memosURL)

	req, err := http.NewRequest(http.MethodPost, endpoint, bodyReader)
	if err != nil {
		fmt.Println("Failed to create request:", err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+os.Getenv("MEMOS_ACCESS_TOKEN"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}

	if res.StatusCode != http.StatusOK {
		fmt.Printf("Returned status: %d\n", res.StatusCode)
		return
	}

	var createdSpace Space
	if err := json.NewDecoder(res.Body).Decode(&createdSpace); err != nil {
		fmt.Println("Failed to parse space response:", err)
	}

	for _, channel := range uncategorizedChannels {
		transferChannel(s, channel, activeThreadMap, "", &createdSpace)
	}

	for _, category := range categoryObjects {
		fmt.Printf("Category: %s\n", category.Name)

		categoryTag := formatTag(category.Name)

		for _, channel := range categoryToChannels[category.ID] {
			transferChannel(s, channel, activeThreadMap, categoryTag, &createdSpace)
		}
	}
}

func transferChannel(s *discordgo.Session, channel *discordgo.Channel, threadMap map[string][]*discordgo.Channel, tag string, space *Space) {
	fmt.Printf("Channel name: %s\n", channel.Name)

	channelTag := formatTag(channel.Name)
	if tag != "" {
		channelTag = fmt.Sprintf("%s/%s", tag, channelTag)
	}

	sendMessagesToMemos(s, channel, channelTag, space)

	if channel.Type == discordgo.ChannelTypeGuildText || channel.Type == discordgo.ChannelTypeGuildNews || channel.Type == discordgo.ChannelTypeGuildForum {
		for _, thread := range threadMap[channel.ID] {
			transferThread(s, thread, channelTag, space)
		}

		var before *time.Time
		for {
			archived, err := s.ThreadsArchived(channel.ID, before, 100)
			if err != nil {
				fmt.Println("Error fetching archived threads:", err)
				break
			}

			for _, thread := range archived.Threads {
				transferThread(s, thread, channelTag, space)
			}

			if !archived.HasMore {
				break
			}

			lastThread := archived.Threads[len(archived.Threads)-1]
			before = &lastThread.ThreadMetadata.ArchiveTimestamp
		}
	}
}

func transferThread(s *discordgo.Session, thread *discordgo.Channel, tag string, space *Space) {
	fmt.Printf("    ↳ [%s]\n", thread.Name)

	threadTag := formatTag(thread.Name)
	newTag := fmt.Sprintf("%s/%s", tag, threadTag)

	sendMessagesToMemos(s, thread, newTag, space)
}

func sendMessagesToMemos(s *discordgo.Session, channel *discordgo.Channel, tag string, space *Space) {
	messageList := getSortedMessages(s, channel)

	if len(messageList) == 0 {
		return
	}

	for _, m := range messageList {
		if m.Content == "" && len(m.Attachments) == 0 {
			continue
		}

		var memoAttachments []Attachment

		for _, attachment := range m.Attachments {
			res, err := uploadAttachmentToMemos(attachment.URL, attachment.Filename)
			if err != nil {
				fmt.Printf("Failed to upload attachment %s: %v\n", attachment.Filename, err)
				continue
			}
			memoAttachments = append(memoAttachments, *res)
		}

		content := fmt.Sprintf("%s #%s", m.Content, tag)

		err := createMemo(content, memoAttachments, space)
		if err != nil {
			fmt.Println("Failed to create memo:", err)
		}
	}
}

func createMemo(content string, attachments []Attachment, space *Space) error {
	payload := MemoPayload{
		Content:     content,
		Attachments: attachments,
		Space:       space.Name,
	}

	endpoint := fmt.Sprintf("%s/api/v1/memos", memosURL)

	for attempt := 1; attempt <= 10; attempt++ {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("Failed to marshal JSON: %w", err)
		}

		bodyReader := bytes.NewBuffer(jsonData)

		req, err := http.NewRequest(http.MethodPost, endpoint, bodyReader)
		if err != nil {
			return fmt.Errorf("Failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+os.Getenv("MEMOS_ACCESS_TOKEN"))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("Error sending request: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			break
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()

			retryAfterStr := resp.Header.Get("Retry-After")
			waitSeconds := 1

			if retryAfterStr == "" {
				if parsedStr, parseErr := strconv.Atoi(retryAfterStr); parseErr == nil {
					waitSeconds = parsedStr
				}
			} else {
				waitSeconds = 1 << (attempt - 1)
			}

			//fmt.Printf("Retrying in %d seconds...\n", waitSeconds)
			time.Sleep(time.Duration(waitSeconds) * time.Second)
			continue
		}

		defer resp.Body.Close()
		return fmt.Errorf("Returned status: %d", resp.StatusCode)
	}

	return nil
}

func formatTag(str string) string {
	words := strings.Fields(str)
	joined := strings.Join(words, "-")
	lowerCase := strings.ToLower(joined)

	var builder strings.Builder

	for _, c := range lowerCase {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			builder.WriteRune(c)
		} else {
			switch c {
			case '-', '`', '~', '$', '^', '&', '_', '+', '=', '|', '<', '>':
				builder.WriteRune(c)
			case '/':
				builder.WriteRune('-')
			default:
			}
		}
	}

	result := builder.String()
	return result
}

func uploadAttachmentToMemos(url, filename string) (*Attachment, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Failed to download: %w", err)
	}
	defer resp.Body.Close()

	// discord attachment in bytes
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed to read response body: %w", err)
	}

	attachment := Attachment{
		Filename: filename,
		Content:  data,
		Type:     resp.Header.Get("Content-Type"),
	}

	jsonData, err := json.Marshal(attachment)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal JSON: %w", err)
	}

	bodyReader := bytes.NewBuffer(jsonData)

	endpoint := fmt.Sprintf("%s/api/v1/attachments", memosURL)

	req, err := http.NewRequest(http.MethodPost, endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+os.Getenv("MEMOS_ACCESS_TOKEN"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	uploadResp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Memos upload failed: %w", err)
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Returned status: %d", uploadResp.StatusCode)
	}

	var createdAttachment Attachment
	if err := json.NewDecoder(uploadResp.Body).Decode(&createdAttachment); err != nil {
		return nil, fmt.Errorf("Failed to parse attachment response: %w", err)
	}

	return &createdAttachment, nil
}
