package main

import (
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	log "github.com/Sirupsen/logrus"
	msg "github.com/UniversityRadioYork/bifrost-go/message"
	"github.com/UniversityRadioYork/ury-listd/tcp"
	"github.com/docopt/docopt-go"
)

//var log = logrus.New()

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

type cfgType struct {
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

func main() {
	// Get arguments (or their defaults)
	args, err := parseArgs(os.Args[0])
	if err != nil {
		log.Fatal("Error parsing args: ", err.Error())
	}

	// Parse config
	var cfg cfgType
	if _, err := toml.DecodeFile(args["--config"].(string), &cfg); err != nil {
		log.Fatal("Error decoding toml config: ", err.Error())
	}

	// Properly set up logger
	if cfg.Log.Level != "" {
		level, err := log.ParseLevel(cfg.Log.Level)
		if err != nil {
			log.Fatal("Failed to parse log level: ", err.Error())
		}
		log.SetLevel(level)
	}

	playoutReceive := make(chan msg.Message)
	playoutSend := make(chan msg.Message)
	go tcp.PlayoutConn(cfg.Playout.URI, playoutReceive, playoutSend)

	serverReceive := make(chan msg.Message)
	server := tcp.NewServer(cfg.Server.Listen, serverReceive)
	go server.Listen()

	go ProcessRequest(serverReceive, playoutSend)
	ProcessResponse(playoutReceive, server)
}
