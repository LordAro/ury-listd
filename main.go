package main

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Sirupsen/logrus"
	msg "github.com/UniversityRadioYork/bifrost-go/message"
	"github.com/UniversityRadioYork/bifrost-go/tokeniser"
	"github.com/UniversityRadioYork/ury-listd/tcp"
	"github.com/docopt/docopt-go"
)

var log = logrus.New()

// Version string, provided by linker flags
var LDVersion string

var ProgVersion = "ury-listd " + LDVersion

func parseArgs(argv0 string) (args map[string]interface{}, err error) {
	usage := `{prog}.

Usage:
  {prog} [-c <configfile>]
  {prog} -h
  {prog} -v

Options:
  -c --config=<configfile>   Path to config file [default: config.toml]
  -h --help                  Show this screen.
  -v --version               Show version.`
	usage = strings.Replace(usage, "{prog}", argv0, -1)

	return docopt.Parse(usage, nil, true, ProgVersion, false)
}

func main() {
	// Get arguments (or their defaults)
	args, err := parseArgs(os.Args[0])
	if err != nil {
		log.Fatal("Error parsing args: ", err.Error())
	}

	// Parse config
	// TODO: Make it its own type?
	var cfg struct {
		Server struct {
			Listen string
		}
		Playout struct {
			URI string
		}
		Log struct {
			Level string
		}
	}
	if _, err := toml.DecodeFile(args["--config"].(string), &cfg); err != nil {
		log.Fatal("Error decoding toml config: ", err.Error())
	}

	// Properly set up logger
	if cfg.Log.Level != "" {
		level, err := logrus.ParseLevel(cfg.Log.Level)
		if err != nil {
			log.Fatal("Failed to parse log level: ", err.Error())
		}
		log.Level = level
	}

	peer := make(chan msg.Message)
	go PlayoutConn(cfg.Playout.URI, peer)

	l, err := net.Listen("tcp", cfg.Server.Listen)
	if err != nil {
		log.Fatal("Could not start server: ", err.Error())
	}
	log.Info("Listening on ", l.Addr())

	// Listen for clients
	for {
		c, err := l.Accept()
		if err != nil {
			log.Error("Could not accept connection: ", err.Error())
		}
		go NewServerConn(c, peer)
	}
}

func PlayoutConn(connStr string, peer <-chan msg.Message) {
	playout, err := net.Dial("tcp", connStr)
	defer playout.Close()
	if err != nil {
		log.Fatal("Could not connect to playout: ", err.Error())
	}
	for m := range peer {
		_, err := playout.Write(m.Pack())
		if err != nil {
			log.Error("Error writing message to playout: ", err.Error())
			return // TODO: Reconnect
		}
	}
}

func NewServerConn(c net.Conn, peer chan<- msg.Message) {
	log.Info("New connection from ", c.RemoteAddr())
	defer c.Close()

	if tcp.peers.Add(c.RemoteAddr().String()) == nil {
		log.Error("Duplicate connection")
		return // Connection from the same place? Wat.
	}
	defer tcp.peers.Remove(c.RemoteAddr().String())

	tok := tokeniser.New(c)
	fmt.Fprintln(c, "Much echo") // TODO: DUMP
	for {
		mstr, err := tok.Tokenise()
		if err != nil {
			log.Error("Error reading message: ", err.Error())
			return
		}
		m := msg.Message(mstr)
		peer <- m
		log.Debug("Received: ", []string(m))
	}
}
