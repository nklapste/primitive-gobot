package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"io/ioutil"
	"strings"
	"syscall"
	"github.com/fogleman/primitive/primitive"
	"github.com/bwmarrin/discordgo"
	"net/http"
	"time"
	"bufio"
	"image/png"
	"image"
	"io"
	"github.com/nfnt/resize"
	"runtime"
	"math/rand"
	"bytes"
	"github.com/satori/go.uuid"
	"strconv"
)

func init() {
	flag.StringVar(&token, "t", "", "Bot token")
	flag.StringVar(&tokenFile, "tf", "", "Bot token file path")
	flag.Parse()
}

var token string
var tokenFile string

func main() {
	if tokenFile != "" {
		b, err := ioutil.ReadFile(tokenFile) // just pass the file name
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
	if m.Author.ID == s.State.User.ID {
		return
	}

	if strings.HasPrefix(m.Content, "!primify") {
		c, err := s.State.Channel(m.ChannelID)
		if err != nil {
			// Could not find channel.
			return
		}

		commands := strings.Split(m.Content, " ")
		commands = commands[1:]

		var (
			shapeNumber int
			background  string
			alpha       int
			inputSize   int
			outputSize  int
			mode        int
			workers     int
			repeat      int
		)

		// instancing a FlagSet and capturing its output for parsing arguments sent
		// through discord messages
		var b bytes.Buffer
		writer := bufio.NewWriter(&b)
		f1 := flag.FlagSet{}
		f1.SetOutput(writer)
		f1.Init("primitive-gobot", flag.ContinueOnError)
		f1.IntVar(&shapeNumber, "n", 25, "number of primitives")
		f1.StringVar(&background, "bg", "", "background color (hex)")
		f1.IntVar(&alpha, "a", 128, "alpha value")
		f1.IntVar(&inputSize, "r", 256, "resize large input images to this size")
		f1.IntVar(&outputSize, "s", 1024, "output image size")
		f1.IntVar(&mode, "m", 1, "0=combo 1=triangle 2=rect 3=ellipse 4=circle 5=rotatedrect 6=beziers 7=rotatedellipse 8=polygon")
		f1.IntVar(&workers, "j", 0, "number of parallel workers (default uses all cores)")
		f1.IntVar(&repeat, "rep", 0, "add N extra shapes per iteration with reduced search")
		f1.Output()
		err = f1.Parse(commands)
		writer.Flush()

		if err != nil {
			s.ChannelMessageSend(c.ID, "```\n"+b.String()+"```")
		}
		if shapeNumber > 1000 {
			s.ChannelMessageSend(c.ID, "`shapeNumber`: `"+strconv.Itoa(shapeNumber)+"` is too high! Please keep it below `1000`")
			return
		}
		if repeat > 20 {
			s.ChannelMessageSend(c.ID, "`repeat`: `"+strconv.Itoa(repeat)+"` is too high! Please keep it below `20`")
			return
		}

		for _, attachment := range m.Attachments {
			s.ChannelMessageSend(c.ID, "Attempting to primify: "+attachment.Filename)
			// get the attachment
			resp, err := http.Get(attachment.URL)
			if err != nil {
				s.ChannelMessageSend(c.ID, err.Error())
				continue
			}
			// decode the attachment into a image
			inputImage, _, err := image.Decode(resp.Body)
			if err != nil {
				s.ChannelMessageSend(c.ID, err.Error())
				continue
			}

			// convert the image into primitive image
			outputPNG, outputSVG := primify(inputImage, shapeNumber, background, alpha, inputSize, outputSize, mode, workers, repeat)

			// generate a unique id and return the primitive image
			id, _ := uuid.NewV4()
			s.ChannelFileSend(c.ID, id.String()+".png", outputPNG)
			s.ChannelFileSend(c.ID, id.String()+".svg", outputSVG)
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

// api that interacts with "github.com/fogleman/primitive/primitive" to create primitive images
func primify(input image.Image, shapeNumber int, background string, alpha int,
	inputSize int, outputSize int, shapeType int, workers int, extraShapes int) (io.Reader, io.Reader) {
	// seed random number generator
	rand.Seed(time.Now().UTC().UnixNano())

	// determine worker count
	if workers < 1 || workers > runtime.NumCPU() {
		workers = runtime.NumCPU()
	}

	// scale down input image if needed
	size := uint(inputSize)
	if size > 0 {
		input = resize.Thumbnail(size, size, input, resize.Bilinear)
	}

	// determine background color
	var bg primitive.Color
	if background == "" {
		bg = primitive.MakeColor(primitive.AverageImageColor(input))
	} else {
		bg = primitive.MakeHexColor(background)
	}

	// run algorithm
	model := primitive.NewModel(input, bg, outputSize, workers)
	frame := 0

	for i := 0; i < shapeNumber; i++ {
		frame++
		// find optimal shape and add it to the model
		model.Step(primitive.ShapeType(shapeType), alpha, extraShapes)
	}

	// encode the model as a png image and write it to a buffer for later use
	var PNGBuffer bytes.Buffer
	writer := bufio.NewWriter(&PNGBuffer)
	png.Encode(writer, model.Context.Image())
	writer.Flush()

	// also save the model as its raw svg
	var SVGBuffer bytes.Buffer
	SVGBuffer.WriteString(model.SVG())

	// return the output images (PNG, SVG) as a Readers for later use
	return bufio.NewReader(&PNGBuffer), bufio.NewReader(&SVGBuffer)
}
