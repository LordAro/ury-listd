package main

import (
	log "github.com/Sirupsen/logrus"
	msg "github.com/UniversityRadioYork/bifrost-go/message"
	"github.com/UniversityRadioYork/ury-listd/tcp"
)

func ProcessResponse(playoutReceive <-chan msg.Message, server *tcp.Server) {
	for m := range playoutReceive {
		log.Debug("Received from playout: `", m, "`")
		// Validate message
		// If our tag, process
		// If broadcast msg, process & pass on
		server.Broadcast(m)
	}
}

func ProcessRequest(serverReceive <-chan msg.Message, playoutSend chan<- msg.Message) {
	for m := range serverReceive {
		log.Debug("Received from client: `", m, "`")
		// Validate message
		// If is for listd, process
		// Send through to playout otherwise
		playoutSend <- m
	}
}
