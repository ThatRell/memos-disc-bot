package main

import (
	"fmt"
	"sort"

	"github.com/bwmarrin/discordgo"
)

func parseServer(s *discordgo.Session, g *discordgo.Guild) (map[string][]*discordgo.Channel, []*discordgo.Channel, []*discordgo.Channel, map[string][]*discordgo.Channel, error) {
	var channels []*discordgo.Channel
	var err error

	// fetch from cache
	if len(g.Channels) > 0 {
		channels = make([]*discordgo.Channel, len(g.Channels))
		copy(channels, g.Channels)
	} else {
		// fallback to fetch from discord API
		channels, err = s.GuildChannels(g.ID)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("Error fetching channels: %w", err)
		}
	}

	// first group channels by their category
	categoryToChannels := make(map[string][]*discordgo.Channel)
	var categoryObjects []*discordgo.Channel
	var uncategorizedChannels []*discordgo.Channel

	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory {
			categoryObjects = append(categoryObjects, channel)
		} else {
			if channel.ParentID != "" {
				categoryToChannels[channel.ParentID] = append(categoryToChannels[channel.ParentID], channel)
			} else {
				uncategorizedChannels = append(uncategorizedChannels, channel)
			}
		}
	}

	// then sort the categories
	sort.Slice(categoryObjects, func(i, j int) bool {
		return categoryObjects[i].Position < categoryObjects[j].Position
	})

	sort.Slice(uncategorizedChannels, func(i, j int) bool {
		groupI := getSortGroup(uncategorizedChannels[i].Type)
		groupJ := getSortGroup(uncategorizedChannels[j].Type)

		if groupI != groupJ {
			return groupI < groupJ
		}
		return uncategorizedChannels[i].Position < uncategorizedChannels[j].Position
	})

	// then sort the channels inside them
	for _, childChannels := range categoryToChannels {
		sort.Slice(childChannels, func(i, j int) bool {
			groupI := getSortGroup(childChannels[i].Type)
			groupJ := getSortGroup(childChannels[j].Type)

			if groupI != groupJ {
				return groupI < groupJ
			}
			return childChannels[i].Position < childChannels[j].Position
		})
	}

	// fetch threads
	activeThreadMap := make(map[string][]*discordgo.Channel)
	activeThreads, err := s.GuildThreadsActive(g.ID)
	if err == nil {
		for _, thread := range activeThreads.Threads {
			activeThreadMap[thread.ParentID] = append(activeThreadMap[thread.ParentID], thread)
		}
	}

	return categoryToChannels, categoryObjects, uncategorizedChannels, activeThreadMap, nil
}

func getSortGroup(cType discordgo.ChannelType) int {
	if cType == discordgo.ChannelTypeGuildVoice || cType == discordgo.ChannelTypeGuildStageVoice {
		return 1
	}
	return 0
}

func getSortedMessages(s *discordgo.Session, channel *discordgo.Channel) []*discordgo.Message {
	var messageList []*discordgo.Message

	beforeID := ""
	for {
		messages, err := s.ChannelMessages(channel.ID, 100, beforeID, "", "")
		if err != nil {
			fmt.Println("Error fetching messages:", err)
			break
		}

		if len(messages) == 0 {
			break
		}

		messageList = append(messageList, messages...)
		beforeID = messageList[len(messageList)-1].ID
	}

	// reverse message list
	for i, j := 0, len(messageList)-1; i < j; i, j = i+1, j-1 {
		messageList[i], messageList[j] = messageList[j], messageList[i]
	}

	return messageList
}
