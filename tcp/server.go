package tcp

import (
	"net"

	log "github.com/Sirupsen/logrus"
	msg "github.com/UniversityRadioYork/bifrost-go/message"
)

type Server struct {
	connStr     string
	receive     chan<- msg.Message
	connections *Peers
}

func NewServer(connStr string, receive chan<- msg.Message) *Server {
	return &Server{
		connStr:     connStr,
		receive:     receive,
		connections: &Peers{m: make(map[string]chan<- msg.Message)},
	}
}

func (s *Server) Listen() {
	l, err := net.Listen("tcp", s.connStr)
	if err != nil {
		log.Fatal("Could not start server: ", err.Error())
	}
	log.Info("Listening on ", l.Addr())

	for {
		c, err := l.Accept()
		if err != nil {
			log.Error("Could not accept connection: ", err.Error())
		}
		go s.RunClientConn(c)
	}
}

func (s *Server) Broadcast(m msg.Message) {
	for _, v := range s.connections.Map() {
		select {
		case v <- m:
			// TODO: Add timeout?
		}
	}
}

func (s *Server) RunClientConn(c net.Conn) {
	log.Info("New connection from ", c.RemoteAddr())
	defer c.Close()

	send := s.connections.Add(c.RemoteAddr().String())
	if send == nil {
		log.Error("Duplicate connection")
		return // Connection from the same place? Wat.
	}

	ConnReadWriteAsync(c, s.receive, send)

	log.Info("Removing foo connection ", c.RemoteAddr().String())
	s.connections.Remove(c.RemoteAddr().String())

}
