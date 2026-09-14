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
		fmt.Println("error loading .env file,", err)
	}

	dg, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	dg.AddHandler(ParseServers)

	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	dg.Close()
}

func ParseServers(s *discordgo.Session, r *discordgo.Ready) {
	fmt.Println("Bot is connected to the following servers:")

	for _, guild := range s.State.Guilds {
		fmt.Printf("Guild Name: %s, Guild ID: %s\n", guild.Name, guild.ID)
		ParseServer(s, guild)
	}
}

func ParseServer(s *discordgo.Session, g *discordgo.Guild) {
	channels, err := s.GuildChannels(g.ID)
	if err != nil {
		fmt.Println("error fetching channels,", err)
		return
	}

	for _, channel := range channels {
		switch channel.Type {
		case discordgo.ChannelTypeGuildCategory:
			fmt.Printf("Category: %s (ID: %s)\n", channel.Name, channel.ID)
		case discordgo.ChannelTypeGuildText:
			fmt.Printf("TextChannel: %s (ID: %s)\n", channel.Name, channel.ID)
		case discordgo.ChannelTypeGuildPublicThread:
			fmt.Printf("Thread: %s (ID: %s)\n", channel.Name, channel.ID)
		default:
			fmt.Printf("None: %s (ID: %s)\n", channel.Name, channel.ID)
		}
	}
}
