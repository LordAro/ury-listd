package main

import (
	"bufio"
	"log"
	"net"

	"github.com/UniversityRadioYork/bifrost-go"
)

// Client is a connection to our TCP server.
type Client struct {
	net.Conn
	tokeniser *bifrost.Tokeniser
}

// Tuple type for sending client and an error down a channel.
type clientError struct {
	c *Client
	e error
}

// Tuple type for sending client and a message down a channel.
type clientMessage struct {
	c *Client
	m bifrost.Message
}

func (c *Client) listen(rmCh chan<- clientError, msgCh chan<- clientMessage) {
	c.tokeniser = bifrost.NewTokeniser()
	reader := bufio.NewReader(c)
	for {
		data, err := reader.ReadBytes('\n')
		if err != nil {
			rmCh <- clientError{c, err} // Remove self
			return
		}
		lines, _, err := c.tokeniser.Tokenise(data)
		if err != nil {
			rmCh <- clientError{c, err} // Remove self
			return
		}
		for _, line := range lines {
			msg, err := bifrost.LineToMessage(line)
			if err != nil {
				rmCh <- clientError{c, err} // Remove self
				return
			}
			msgCh <- clientMessage{c, *msg}
		}
	}
}

// Send asyncronously writes a message string to the client instance.
func (c *Client) Send(message bifrost.Message) {
	// We don't care about any return value (errors handled by listen thread)
	// so run this in a goroutine to stop slow things (e.g. networks) slowing
	// the whole program down.
	go func() {
		data, err := message.Pack()
		if err != nil {
			log.Println(err)
		}
		_, err = c.Write(data)
		if err != nil {
			// If error, reasonable to assume client has been removed by listen
			// gorountine. No need to do anything else.
			log.Println(err)
		}
	}()
}
