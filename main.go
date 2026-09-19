package main

import (
	"fmt"
	"os"
	"os/signal"
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
		return
	}

	if m.Content == "-download" || m.Content == "-transfer" {
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

		if m.Content == "-transfer" {
			s.ChannelMessageSend(m.ChannelID, "Transferring server data...")
		} else {
			s.ChannelMessageSend(m.ChannelID, "Downloading server data...")
			downloadServer(s, guild)
		}
	}
}
