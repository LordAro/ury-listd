package main

import (
	"net"

	"github.com/Sirupsen/logrus"
	"github.com/UniversityRadioYork/bifrost-go"
)

// Context is the main listd instance. Manages TCP connections (including the
// connector) and controls the Playlist instance.
type Context struct {
	log *logrus.Logger
	pl  *Playlist

	// TCP Stuffs
	addr         string
	clients      map[*Client]bool
	outgoingAddr string
	outgoing     *Client
}

func (ctx *Context) onClientConnect(c *Client) {
	ctx.log.Info("New client connection", c.RemoteAddr())
}

func (ctx *Context) onClientDisconnect(c *Client, err error) {
	clErr := c.Close()
	if clErr != nil {
		// Err, help?
		ctx.log.Warn("Error closing connection: ", clErr)
	}
	delete(ctx.clients, c)
	ctx.log.Warn("Connection lost from ", c.RemoteAddr())
}

func (ctx *Context) onNewRequest(c *Client, message bifrost.Message) {
	ctx.log.Debug("Request: ", message.String())
	ctx.outgoing.Send(message)
}

func (ctx *Context) onNewResponse(message bifrost.Message) {
	ctx.log.Debug("Response: ", message.String())
	ctx.Broadcast(message)
}

// Broadcast sends a message to all connected clients.
func (ctx *Context) Broadcast(message bifrost.Message) {
	for client := range ctx.clients {
		client.Send(message)
	}
}

func (ctx *Context) newClient(conn net.Conn, rmCh chan<- clientError, requestCh chan<- clientMessage) *Client {
	client := &Client{
		Conn: conn,
	}
	go client.listen(rmCh, requestCh)
	return client
}

func (ctx *Context) listenNewConnections(newConn chan<- net.Conn) {
	listener, err := net.Listen("tcp", ctx.addr)
	if err != nil {
		ctx.log.Fatal(err)
	}
	ctx.log.Info("Listening on ", ctx.addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			ctx.log.Warn(err)
		}
		newConn <- conn
	}
}

// Run the main loop (handling channels).
func (ctx *Context) Run() {
	newCh := make(chan net.Conn)
	go ctx.listenNewConnections(newCh)

	// Make connection
	responseCh := make(chan clientMessage)
	outgoingRmCh := make(chan clientError)
	conn, err := net.Dial("tcp", ctx.outgoingAddr)
	if err != nil {
		ctx.log.Fatal(err)
	}
	ctx.outgoing = ctx.newClient(conn, outgoingRmCh, responseCh)

	// Main loop
	requestCh := make(chan clientMessage)
	rmCh := make(chan clientError)
	for {
		select {
		// Client stuff
		case conn := <-newCh:
			client := ctx.newClient(conn, rmCh, requestCh)
			ctx.clients[client] = true
			ctx.onClientConnect(client)
		case clienterr := <-rmCh:
			ctx.onClientDisconnect(clienterr.c, clienterr.e)
		case request := <-requestCh:
			ctx.onNewRequest(request.c, request.m)

		// Outgoing stuff
		case response := <-responseCh:
			// Don't care about the client, we already know about that
			ctx.onNewResponse(response.m)
		case outgoingerr := <-outgoingRmCh:
			// Connection died :(
			// TODO: Handle it
			ctx.log.Fatal(outgoingerr.e)
			// TODO: Stop the world.
		}
	}
}

// NewContext creates a new context that will listen for clients on hostport,
// make an outgoing connection on outgoing and make use of logger for logging.
func NewContext(hostport, outgoing string, logger *logrus.Logger) *Context {
	c := &Context{
		addr:         hostport,
		outgoingAddr: outgoing,
		clients:      make(map[*Client]bool),
		log:          logger,
	}
	return c
}
