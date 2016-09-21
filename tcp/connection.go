package tcp

import (
	"net"

	msg "github.com/UniversityRadioYork/bifrost-go/message"
	"github.com/UniversityRadioYork/bifrost-go/tokeniser"
)

func ConnReadWriteAsync(c net.Conn, receive chan<- msg.Message, send <-chan msg.Message) error {
	go WriteToConn(c, send)

	if err := ReadFromConn(c, receive); err != nil {
		return err
	}
	return nil
}

func ReadFromConn(c net.Conn, receive chan<- msg.Message) error {
	tok := tokeniser.New(c)
	for {
		mstr, err := tok.Tokenise()
		if err != nil {
			return err
		}
		m := msg.Message(mstr)
		receive <- m
	}
}

func WriteToConn(c net.Conn, send <-chan msg.Message) error {
	for {
		m, ok := <-send
		if !ok {
			// Have been killed
			break
		}
		_, err := c.Write(m.Pack())
		if err != nil {
			return err
		}
	}
	return nil
}
