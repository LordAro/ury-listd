package main

import (
	"log"
	"net"
	"strconv"

	baps3 "github.com/UniversityRadioYork/baps3-go"
)

type clientAndMessage struct {
	c   *Client
	msg baps3.Message
}

// Maintains communications with the downstream service and connected clients.
// Also does any processing needed with the commands.
type hub struct {
	// All current clients.
	clients map[*Client]bool

	// Downstream service state
	downstreamState baps3.ServiceState

	autoAdvance bool

	// Playlist instance
	pl *Playlist

	// For communication with the downstream service.
	cReqCh chan<- baps3.Message
	cResCh <-chan baps3.Message

	// Where new requests from clients come through.
	reqCh chan clientAndMessage

	// Handlers for adding/removing connections.
	addCh chan *Client
	rmCh  chan *Client
	Quit  chan bool
}

// Handles a new client connection.
// conn is the new connection object.
func (h *hub) handleNewConnection(conn net.Conn) {
	defer conn.Close()
	client := &Client{
		conn:  conn,
		resCh: make(chan baps3.Message),
		tok:   baps3.NewTokeniser(),
	}

	// Register user
	h.addCh <- client

	go client.Read(h.reqCh, h.rmCh)
	client.Write(client.resCh, h.rmCh)
}

//
// Request handler
//

func makeBadCommandMsgs() []*baps3.Message {
	return []*baps3.Message{baps3.NewMessage(baps3.RsWhat).AddArg("Bad command")}
}

// Appends the downstream service's version (from the OHAI) to the listd version.
func (h *hub) makeRsOhai() *baps3.Message {
	return baps3.NewMessage(baps3.RsOhai).AddArg("listd " + LD_VERSION + "/" + h.downstreamState.Identifier)
}

// Crafts the features message by adding listd's features to the downstream service's and removing
// features listd intercepts.
func (h *hub) makeRsFeatures() (msg *baps3.Message) {
	features := h.downstreamState.Features
	features.DelFeature(baps3.FtFileLoad) // 'Mask' the features listd intercepts
	features.AddFeature(baps3.FtPlaylist)
	features.AddFeature(baps3.FtPlaylistTextItems)
	features.AddFeature(baps3.FtPlaylistAutoAdvance)
	msg = features.ToMessage()
	return
}

func (h *hub) makeRsAutoAdvance() (msg *baps3.Message) {
	var autoadvancestate string
	if h.autoAdvance {
		autoadvancestate = "on"
	} else {
		autoadvancestate = "off"
	}
	return baps3.NewMessage(baps3.RsAutoAdvance).AddArg(autoadvancestate)
}

// Collates all the responses that comprise a dump response.
// Exists as this is used by the dump response handler /and/ is sent on client connection
func (h *hub) makeDumpResponses() (msgs []*baps3.Message) {
	msgs = append(msgs, baps3.NewMessage(baps3.RsState).AddArg(h.downstreamState.State.String()))
	if h.downstreamState.State != baps3.StEjected {
		msgs = append(msgs, baps3.NewMessage(baps3.RsTime).AddArg(
			strconv.FormatInt(h.downstreamState.Time.Nanoseconds()/1000, 10)))
	}
	msgs = append(msgs, h.makeRsAutoAdvance())
	msgs = append(msgs, h.makeListResponses()...)
	return
}

// Collates all the responses that comprise a list reponse.
// Exists as this is used by the list response handler and makeDumpResponse.
func (h *hub) makeListResponses() (msgs []*baps3.Message) {
	msgs = append(msgs, baps3.NewMessage(baps3.RsCount).AddArg(strconv.Itoa(len(h.pl.items))))
	for i, item := range h.pl.items {
		typeStr := "file"
		if !item.IsFile {
			typeStr = "text"
		}
		msgs = append(msgs, baps3.NewMessage(baps3.RsItem).AddArg(strconv.Itoa(i)).AddArg(item.Hash).AddArg(typeStr).AddArg(item.Data))
	}
	return
}

func sendInvalidCmd(c *Client, errRes baps3.Message, oldCmd baps3.Message) {
	for _, w := range oldCmd.AsSlice() {
		errRes.AddArg(w)
	}
	c.resCh <- errRes
}

func (h *hub) processReqDequeue(req baps3.Message) (resps []*baps3.Message) {
	args := req.Args()
	if len(args) != 2 {
		return makeBadCommandMsgs()
	}
	iStr, hash := args[0], args[1]

	i, err := strconv.Atoi(iStr)
	if err != nil {
		return append(resps, baps3.NewMessage(baps3.RsWhat).AddArg("Bad index"))
	}

	oldSelection := h.pl.selection
	rmIdx, rmHash, err := h.pl.Dequeue(i, hash)
	if err != nil {
		return append(resps, baps3.NewMessage(baps3.RsFail).AddArg(err.Error()))
	}
	if oldSelection != h.pl.selection {
		if !h.pl.HasSelection() {
			resps = append(resps, baps3.NewMessage(baps3.RsSelect))
		} else {
			resps = append(resps, baps3.NewMessage(baps3.RsSelect).AddArg(strconv.Itoa(h.pl.selection)).AddArg(h.pl.items[h.pl.selection].Hash))
		}
	}
	return append(resps, baps3.NewMessage(baps3.RsDequeue).AddArg(strconv.Itoa(rmIdx)).AddArg(rmHash))
}

func (h *hub) processReqEnqueue(req baps3.Message) (resps []*baps3.Message) {
	args := req.Args()
	if len(args) != 4 {
		return makeBadCommandMsgs()
	}
	iStr, hash, itemType, data := args[0], args[1], args[2], args[3]

	i, err := strconv.Atoi(iStr)
	if err != nil {
		return append(resps, baps3.NewMessage(baps3.RsWhat).AddArg("Bad index"))
	}

	if itemType != "file" && itemType != "text" {
		return append(resps, baps3.NewMessage(baps3.RsWhat).AddArg("Bad item type"))
	}

	oldSelection := h.pl.selection
	item := &PlaylistItem{Data: data, Hash: hash, IsFile: itemType == "file"}
	newIdx, err := h.pl.Enqueue(i, item)
	if err != nil {
		return append(resps, baps3.NewMessage(baps3.RsFail).AddArg(err.Error()))
	}
	if oldSelection != h.pl.selection {
		resps = append(resps, baps3.NewMessage(baps3.RsSelect).AddArg(strconv.Itoa(h.pl.selection)).AddArg(h.pl.items[h.pl.selection].Hash))
	}
	return append(resps, baps3.NewMessage(baps3.RsEnqueue).AddArg(strconv.Itoa(newIdx)).AddArg(item.Hash).AddArg(itemType).AddArg(item.Data))
}

func (h *hub) processReqSelect(req baps3.Message) (resps []*baps3.Message) {
	args := req.Args()
	if len(args) == 0 {
		if h.pl.HasSelection() {
			// Remove current selection
			h.cReqCh <- *baps3.NewMessage(baps3.RqEject)
			h.pl.selection = -1
			resps = append(resps, baps3.NewMessage(baps3.RsSelect))
		} else {
			// TODO: Should we care about there not being an existing selection?
			resps = append(resps, baps3.NewMessage(baps3.RsFail).AddArg("No selection to remove"))
		}
	} else if len(args) == 2 {
		iStr, hash := args[0], args[1]

		i, err := strconv.Atoi(iStr)
		if err != nil {
			return append(resps, baps3.NewMessage(baps3.RsWhat).AddArg("Bad index"))
		}

		newIdx, newHash, err := h.pl.Select(i, hash)
		if err != nil {
			return append(resps, baps3.NewMessage(baps3.RsFail).AddArg(err.Error()))
		}

		h.cReqCh <- *baps3.NewMessage(baps3.RqLoad).AddArg(h.pl.items[h.pl.selection].Data)
		resps = append(resps, baps3.NewMessage(baps3.RsSelect).AddArg(strconv.Itoa(newIdx)).AddArg(newHash))
	} else {
		resps = makeBadCommandMsgs()
	}
	return
}

func (h *hub) processReqList(req baps3.Message) (resps []*baps3.Message) {
	resps = h.makeListResponses()
	return
}

func (h *hub) processReqLoadEject(req baps3.Message) (resps []*baps3.Message) {
	return makeBadCommandMsgs()
}

func (h *hub) processReqDump(req baps3.Message) (msgs []*baps3.Message) {
	return h.makeDumpResponses()
}

func (h *hub) processReqAutoadvance(req baps3.Message) (msgs []*baps3.Message) {
	if len(req.Args()) != 1 {
		return makeBadCommandMsgs()
	}
	onoff, _ := req.Arg(0)
	switch onoff {
	case "on":
		h.autoAdvance = true
	case "off":
		h.autoAdvance = false
	default:
		return append(msgs, baps3.NewMessage(baps3.RsWhat).AddArg("Bad argument"))
	}
	return append(msgs, h.makeRsAutoAdvance())
}

var REQ_FUNC_MAP = map[baps3.MessageWord]func(*hub, baps3.Message) []*baps3.Message{
	baps3.RqEnqueue:     (*hub).processReqEnqueue,
	baps3.RqDequeue:     (*hub).processReqDequeue,
	baps3.RqSelect:      (*hub).processReqSelect,
	baps3.RqList:        (*hub).processReqList,
	baps3.RqLoad:        (*hub).processReqLoadEject,
	baps3.RqEject:       (*hub).processReqLoadEject,
	baps3.RqDump:        (*hub).processReqDump,
	baps3.RqAutoAdvance: (*hub).processReqAutoadvance,
}

// Handles a request from a client.
// Falls through to the connector cReqCh if command is "not understood".
func (h *hub) processRequest(c *Client, req baps3.Message) {
	log.Println("New request:", req.String())
	if reqFunc, ok := REQ_FUNC_MAP[req.Word()]; ok {
		responses := reqFunc(h, req)
		for _, resp := range responses {
			// TODO: Add a "is fail word" func to baps3-go?
			if resp.Word() == baps3.RsFail || resp.Word() == baps3.RsWhat {
				// failures only go to sender
				sendInvalidCmd(c, *resp, req)
			} else {
				h.broadcast(*resp)
			}
		}
	} else {
		h.cReqCh <- req
	}
}

//
// Response handler
//

func (h *hub) handleRsEnd(res baps3.Message) {
	if h.autoAdvance && h.pl.Advance() { // Selection changed
		h.cReqCh <- *baps3.NewMessage(baps3.RqLoad).AddArg(h.pl.items[h.pl.selection].Data)
		h.broadcast(*baps3.NewMessage(baps3.RsSelect).AddArg(strconv.Itoa(h.pl.selection)))
	}
}

// Processes a response from the downstream service.
func (h *hub) processResponse(res baps3.Message) {
	log.Println("New response:", res.String())
	switch res.Word() {
	case baps3.RsEnd: // Handle, broadcast and update state
		h.handleRsEnd(res)
		fallthrough
	case baps3.RsTime, baps3.RsState: // Broadcast _AND_ update state
		h.broadcast(res)
		fallthrough
	case baps3.RsOhai, baps3.RsFeatures: // Just update state
		if err := h.downstreamState.Update(res); err != nil {
			log.Fatal("Error updating state: " + err.Error())
		}
	default:
		h.broadcast(res)
	}
}

// Send a response message to all clients.
func (h *hub) broadcast(res baps3.Message) {
	for c, _ := range h.clients {
		c.resCh <- res
	}
}

// Listens for new connections on addr:port and spins up the relevant goroutines.
func (h *hub) runListener(addr string, port string) {
	netListener, err := net.Listen("tcp", addr+":"+port)
	if err != nil {
		log.Println("Listening error:", err.Error())
		return
	}

	// Get new connections
	go func() {
		for {
			conn, err := netListener.Accept()
			if err != nil {
				log.Println("Error accepting connection:", err.Error())
				continue
			}

			go h.handleNewConnection(conn)
		}
	}()

	for {
		select {
		case msg := <-h.cResCh:
			h.processResponse(msg)
		case data := <-h.reqCh:
			h.processRequest(data.c, data.msg)
		case client := <-h.addCh:
			h.clients[client] = true
			client.resCh <- *h.makeRsOhai()
			client.resCh <- *h.makeRsFeatures()
			for _, msg := range h.makeDumpResponses() {
				client.resCh <- *msg
			}
			log.Println("New connection from", client.conn.RemoteAddr())
		case client := <-h.rmCh:
			close(client.resCh)
			delete(h.clients, client)
			log.Println("Closed connection from", client.conn.RemoteAddr())
		case <-h.Quit:
			log.Println("Closing all connections")
			for c, _ := range h.clients {
				close(c.resCh)
				delete(h.clients, c)
			}
			//			h.Quit <- true
		}
	}
}

// Sets up the connector channels for the hub object.
func (h *hub) setConnector(cReqCh chan<- baps3.Message, cResCh <-chan baps3.Message) {
	h.cReqCh = cReqCh
	h.cResCh = cResCh
}
