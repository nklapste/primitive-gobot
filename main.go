package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"io/ioutil"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"net/http"
)

func init() {
	flag.StringVar(&token, "t", "", "Bot Token")
	flag.StringVar(&tokenfile, "tf", "", "Bot token file path")
	flag.Parse()
}

var token string
var tokenfile string

func main() {

	if tokenfile != "" {
		b, err := ioutil.ReadFile(tokenfile) // just pass the file name
		if err != nil {
			fmt.Print(err)
			return
		}
		token = string(b)
	}
	if token == "" {
		fmt.Println("No token provided. Please run: primitive-gobot -t <bot token> or  primitive-gobot -tf <bot token file path>")
		return
	}

	// Create a new Discord session using the provided bot token.
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Error creating Discord session: ", err)
		return
	}

	// Register ready as a callback for the ready events.
	dg.AddHandler(ready)

	// Register messageCreate as a callback for the messageCreate events.
	dg.AddHandler(messageCreate)

	// Register guildCreate as a callback for the guildCreate events.
	dg.AddHandler(guildCreate)

	// Open the websocket and begin listening.
	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening Discord session: ", err)
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("primitive-gobot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

// This function will be called (due to AddHandler above) when the bot receives
// the "ready" event from Discord.
func ready(s *discordgo.Session, event *discordgo.Ready) {
	s.UpdateStatus(0, "with themselves")
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	if strings.HasPrefix(m.Content, "!primify") {
		c, err := s.State.Channel(m.ChannelID)
		if err != nil {
			// Could not find channel.
			return
		}

		for _, attachment := range m.Attachments{
			fmt.Printf("Getting attachment %s", attachment.URL)

			// get the attachment
			resp, err := http.Get(attachment.URL)
			if err != nil {
				fmt.Print(err.Error())
			}
			// put the primify stub here

			// send the file back
			s.ChannelFileSend(c.ID, "out.png", resp.Body)
		}
	}
}

// This function will be called (due to AddHandler above) every time a new
// guild is joined.
func guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {

	if event.Guild.Unavailable {
		return
	}

	for _, channel := range event.Guild.Channels {
		if channel.ID == event.Guild.ID {
			_, _ = s.ChannelMessageSend(channel.ID, "primitive-gobot is ready!")
			return
		}
	}
}


