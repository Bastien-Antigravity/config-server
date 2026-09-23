package server

// =============================================================================
// ESSENTIAL PROCESS:
// Manages the connection lifecycle for a single authenticated TCP client,
// coordinating framed message reading, request dispatching, and asynchronous write buffering.
//
// DATA FLOW:
// 1. Input: TransportConnection accepted by the SafeSocket listener.
// 2. Logic: Extracts handshake identity, registers a dedicated client mailbox,
//    dispatches protobuf ConfigMsg payloads to core.ProcessRequest, and handles responses.
// 3. Output: Writes serialized response frames and broadcast updates to the socket.
//
// KEY PARAMETERS:
// - sock: SafeSocket transport connection with framing and handshake support.
// - mailbox: Per-client channel buffer preventing slow readers from stalling the server.
// =============================================================================

import (
	"fmt"
	"io"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/core"

	"github.com/Bastien-Antigravity/safe-socket"
	socket_interfaces "github.com/Bastien-Antigravity/safe-socket/src/interfaces"

	"google.golang.org/protobuf/proto"
)

// -----------------------------------------------------------------------------

func (s *Server) handleConnection(sock socket_interfaces.TransportConnection) {
	defer sock.Close()

	// 1. Extract Client Identity from Handshake
	identity := safesocket.GetIdentity(sock)
	if identity == nil {
		s.Logger.Error("Connection does not have a Handshake identity")
		return
	}

	name, _ := identity.FromName()
	rawAddress, _ := identity.FromAddress()

	endpoint := rawAddress
	if endpoint == "" {
		endpoint = sock.RemoteAddr().String()
	}
	clientName := fmt.Sprintf("%s-%s", name, endpoint)

	s.Logger.Info("Client identified: %s (advertised: %s)", clientName, rawAddress)

	// Set a reasonable idle timeout to clean up zombie connections.
	// 10 minutes is a safe balance for configuration synchronization.
	_ = sock.SetIdleTimeout(10 * time.Minute)

	// Initialize Mailbox (tight buffer of 3 messages)
	mailbox := &clientMailbox{
		name:           clientName,
		serviceAddress: rawAddress,
		send:           make(chan []byte, 3),
	}

	s.addListener(clientName, mailbox)
	defer s.removeListener(clientName, mailbox)

	// 2. Start Writer Loop
	go func() {
		for msg := range mailbox.send {
			if _, err := sock.Write(msg); err != nil {
				s.Logger.Error("Write failed to %s: %v", clientName, err)
				_ = sock.Close()
				break
			}
		}
	}()

	// 3. Reader Loop (Main Goroutine)
	// Using ReadMessage() which is provided by safe-socket for robust framing.
	for {
		data, err := sock.ReadMessage()
		if err != nil {
			if err != io.EOF {
				s.Logger.Error("Read error from %s: %v", clientName, err)
			}
			return
		}

		// Handle ConfigMsg
		s.Logger.Info("Processing request from %s (data len: %d)", clientName, len(data))
		response, err := core.ProcessRequest(data, s.Store, s.broadcastUpdate, s.TriggerSave)
		if err != nil {
			s.Logger.Error("Error processing request from %s: %v", clientName, err)
			return
		}

		if response != nil {
			s.Logger.Info("Sending response %v to %s", response.Command, clientName)
			bytes, err := proto.Marshal(response)
			if err != nil {
				s.Logger.Error("Failed to marshal response: %v", err)
				return
			}

			// Hand off the response to the Writer Loop
			select {
			case mailbox.send <- bytes:
				// Response queued
			default:
				s.Logger.Warning("Response mailbox full for %s. Dropping client.", clientName)
				return
			}
		}
	}
}
