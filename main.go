package main

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file:", err)
		return
	}

	dg, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		fmt.Println("Error creating Discord session:", err)
		return
	}

	dg.AddHandler(messageCreate)

	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening connection:", err)
		return
	}

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	dg.Close()
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if m.GuildID == "" {
		s.ChannelMessageSend(m.ChannelID, "leave me alone ! >.<")
		return
	}

	fmt.Printf("Received message in Guild ID: %s\n", m.GuildID)
	if m.Content == "-download" {
		perms, err := s.UserChannelPermissions(m.Author.ID, m.ChannelID)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error verifying permissions.")
			fmt.Println("Error getting permissions:", err)
			return
		}

		if perms&discordgo.PermissionAdministrator == 0 {
			s.ChannelMessageSend(m.ChannelID, "You must be a server Administrator to use this command.")
			return
		}

		s.ChannelMessageSend(m.ChannelID, "Downloading server data...")

		// fetch from cache
		guild, err := s.State.Guild(m.GuildID)
		if err != nil {
			// fallback to fetch from discord API
			guild, err = s.Guild(m.GuildID)
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, "Failed to retrieve server data.")
				fmt.Println("Error fetching guild:", err)
				return
			}
		}

		parseServer(s, guild)
	}
}

func parseServer(s *discordgo.Session, g *discordgo.Guild) {
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
			fmt.Println("Error fetching channels:", err)
			return
		}
	}

	// first group channels by their category
	categoryToChannels := make(map[string][]*discordgo.Channel)
	channelToThreads := make(map[string][]*discordgo.Channel)
	var categoryObjects []*discordgo.Channel
	var uncategorizedChannels []*discordgo.Channel

	for _, channel := range channels {
		switch channel.Type {
		case discordgo.ChannelTypeGuildCategory:
			categoryObjects = append(categoryObjects, channel)
		case discordgo.ChannelTypeGuildPublicThread, discordgo.ChannelTypeGuildPrivateThread:
			channelToThreads[channel.ParentID] = append(channelToThreads[channel.ParentID], channel)
		default:
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

	// parse channels in order
	for _, channel := range uncategorizedChannels {
		parseChannel(channel)
	}

	for _, category := range categoryObjects {
		fmt.Printf("Category: %s\n", category.Name)
		for _, channel := range categoryToChannels[category.ID] {
			parseChannel(channel)
		}
	}
}

func parseChannel(channel *discordgo.Channel) {
	fmt.Printf("Channel name: %s | Channel Type: %v | Channel Position: %d\n", channel.Name, channel.Type, channel.Position)
}

func getSortGroup(cType discordgo.ChannelType) int {
	if cType == discordgo.ChannelTypeGuildVoice || cType == discordgo.ChannelTypeGuildStageVoice {
		return 1
	}
	return 0
}
