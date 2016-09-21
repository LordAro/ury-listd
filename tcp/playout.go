package tcp

import (
	"net"

	log "github.com/Sirupsen/logrus"
	msg "github.com/UniversityRadioYork/bifrost-go/message"
)

func PlayoutConn(connStr string, receive chan<- msg.Message, send <-chan msg.Message) {
	playout, err := net.Dial("tcp", connStr)
	defer playout.Close()
	if err != nil {
		log.Fatal("Could not connect to playout: ", err.Error())
	}

	ConnReadWriteAsync(playout, receive, send)
	log.Warn("Lost connection to playout: ", err)
	// TODO: Attempt reconnect
}
