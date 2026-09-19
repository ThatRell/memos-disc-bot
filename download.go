package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	serverDataDir = "./server-data"
)

func downloadServer(s *discordgo.Session, g *discordgo.Guild) {
	err := os.MkdirAll(serverDataDir, 0777)
	if err != nil {
		fmt.Println("Error making server-data directory:", err)
		return
	}

	serverDir := filepath.Join(serverDataDir, g.Name)
	err = os.RemoveAll(serverDir)
	if err != nil {
		fmt.Println("Error removing previous server files:", err)
		return
	}

	err = os.MkdirAll(serverDir, 0777)
	if err != nil {
		fmt.Println("Error making server directory:", err)
		return
	}

	categoryToChannels, categoryObjects, uncategorizedChannels, activeThreadMap, err := parseServer(s, g)
	if err != nil {
		fmt.Println("Error parsing discord server:", err)
		return
	}

	// download channels in order
	categoryDir := filepath.Join(serverDir, "UNCATEGORIZED-CHANNELS")
	err = os.MkdirAll(categoryDir, 0777)
	if err != nil {
		fmt.Printf("Error making %s directory: %v\n", "Uncategorized", err)
		return
	}

	for _, channel := range uncategorizedChannels {
		downloadChannel(s, channel, activeThreadMap, categoryDir)
	}

	for _, category := range categoryObjects {
		fmt.Printf("Category: %s\n", category.Name)
		categoryDir = filepath.Join(serverDir, category.Name)
		err = os.MkdirAll(categoryDir, 0777)
		if err != nil {
			fmt.Printf("Error making %s directory: %v\n", "Uncategorized", err)
			return
		}

		for _, channel := range categoryToChannels[category.ID] {
			downloadChannel(s, channel, activeThreadMap, categoryDir)
		}
	}
}

func downloadChannel(s *discordgo.Session, channel *discordgo.Channel, threadMap map[string][]*discordgo.Channel, categoryDir string) {
	fmt.Printf("Channel name: %s | Channel Type: %v | Channel Position: %d\n", channel.Name, channel.Type, channel.Position)

	channelDir := filepath.Join(categoryDir, channel.Name)
	err := os.MkdirAll(channelDir, 0777)
	if err != nil {
		fmt.Printf("Error making %s directory: %v\n", channel.Name, err)
		return
	}

	downloadChannelMessages(s, channel, channelDir)

	if channel.Type == discordgo.ChannelTypeGuildText || channel.Type == discordgo.ChannelTypeGuildNews || channel.Type == discordgo.ChannelTypeGuildForum {
		for _, thread := range threadMap[channel.ID] {
			downloadThread(s, thread, channelDir)
		}

		var before *time.Time
		for {
			archived, err := s.ThreadsArchived(channel.ID, before, 100)
			if err != nil {
				fmt.Println("Error fetching archived threads:", err)
				break
			}

			for _, thread := range archived.Threads {
				downloadThread(s, thread, channelDir)
			}

			if !archived.HasMore {
				break
			}

			lastThread := archived.Threads[len(archived.Threads)-1]
			before = &lastThread.ThreadMetadata.ArchiveTimestamp
		}
	}
}

func downloadThread(s *discordgo.Session, thread *discordgo.Channel, channelDir string) {
	fmt.Printf("    ↳ [%s]\n", thread.Name)

	threadDir := filepath.Join(channelDir, thread.Name)
	err := os.MkdirAll(threadDir, 0777)
	if err != nil {
		fmt.Printf("Error making %s directory: %v\n", thread.Name, err)
		return
	}

	downloadChannelMessages(s, thread, threadDir)
}

func downloadChannelMessages(s *discordgo.Session, channel *discordgo.Channel, channelDir string) {
	messageList := getSortedMessages(s, channel)

	if len(messageList) == 0 {
		return
	}

	fileName := filepath.Join(channelDir, fmt.Sprintf("%s.txt", channel.Name))
	attachments, err := writeMessagesToFile(messageList, fileName)
	if err != nil {
		fmt.Printf("Error downloading %s: %v\n", channel.Name, err)
	}

	if len(attachments) == 0 {
		return
	}

	attachDir := filepath.Join(channelDir, "attachments")
	err = os.MkdirAll(attachDir, 0777)
	if err != nil {
		fmt.Println("Error making attechments directory:", err)
		return
	}
	downloadAttachments(attachments, attachDir)
}

func writeMessagesToFile(messageList []*discordgo.Message, filepath string) ([]*discordgo.MessageAttachment, error) {
	file, err := os.Create(filepath)
	if err != nil {
		return nil, fmt.Errorf("Could not create file: %w", err)
	}

	var allAttachments []*discordgo.MessageAttachment

	for _, m := range messageList {
		if len(m.Content) > 0 {
			line := fmt.Sprintf("%s\n\n", m.Content)

			_, err := file.WriteString(line)
			if err != nil {
				return nil, fmt.Errorf("Error writing message to file: %w", err)
			}
		}

		allAttachments = append(allAttachments, m.Attachments...)
	}

	file.Close()
	info, err := os.Stat(filepath)
	if err != nil {
		return nil, fmt.Errorf("Error checking file stats: %w", err)
	}

	if info.Size() == 0 {
		err := os.Remove(filepath)
		if err != nil {
			return nil, fmt.Errorf("Failed to delete empty file: %w", err)
		}
	}

	return allAttachments, nil
}

func downloadAttachments(attachments []*discordgo.MessageAttachment, folderPath string) {
	var wg sync.WaitGroup

	sem := make(chan struct{}, 10)

	for _, attachment := range attachments {
		wg.Add(1)
		sem <- struct{}{}

		go func(id, url, filename string) {
			defer func() { <-sem }()
			defer wg.Done()

			uniqueFileName := fmt.Sprintf("%s_%s", id, filename)
			dest := filepath.Join(folderPath, uniqueFileName)
			downloadAttachmentFile(url, dest)
		}(attachment.ID, attachment.URL, attachment.Filename)
	}

	wg.Wait()
}

func downloadAttachmentFile(url string, destPath string) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Failed to download %s: %v\n", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Bad status downloading %s: %s\n", url, resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		fmt.Printf("Failed to create file %s: %v\n", destPath, err)
		return
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		fmt.Printf("Failed to write data to %s: %v\n", destPath, err)
	}
}
