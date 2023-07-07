package session

import (
	"net"
	"relay/internal/auth"
	"relay/internal/msg"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type SendFunc func(addr *net.UDPAddr, data []byte)

type SessionManager struct {
	addrToSessions map[string]*Session
	roomToSessions map[string]*Session
	sendMessage    SendFunc
	authenticator  *auth.Authenticator
}

func NewManager() *SessionManager {
	authenticator := auth.NewAuthenticator("/path/to/sqlite.db")
	if authenticator == nil {
		return nil
	}
	return &SessionManager{
		addrToSessions: make(map[string]*Session),
		authenticator:  authenticator,
	}
}

func (mgr *SessionManager) SetSendFunc(sendFunc SendFunc) {
	mgr.sendMessage = sendFunc
}

func (mgr *SessionManager) HandlePacket(addr *net.UDPAddr, data []byte) {
	addrStr := addr.String()
	s := mgr.addrToSessions[addrStr]
	if s == nil {
		if msg.IsCreateRoomRequest(data) {
			mgr.handleCreateRoomRequest(addr, data)
		} else if msg.IsJoinRoomRequest(data) {
			mgr.handleJoinRoomRequest(addr, data)
		}
		logrus.Debugf("Received packet from %s, but it isn't CreateRoomRequest/JoinRoomRequest", addrStr)
	} else {
		s.handlePacket(addr, data)
	}
}

func (mgr *SessionManager) handleCreateRoomRequest(addr *net.UDPAddr, data []byte) {
	request := msg.ParseCreateRoomRequest(data)
	if request == nil {
		logrus.Debugf("ParseCreateRoomRequest failed")
		return
	}
	ok := mgr.authenticator.Auth(request.Username, request.Integrity, data[:48])
	if !ok {
		return
	}
	var room string
	for {
		room = uuid.NewString()
		if _, exists := mgr.roomToSessions[room]; exists {
			continue
		}
		break
	}
	s := &Session{
		Room:        room,
		FirstAddr:   addr,
		sendMessage: mgr.sendMessage,
	}
	mgr.addrToSessions[addr.String()] = s
	mgr.roomToSessions[room] = s
	response := msg.NewCreateRoomResponse(room)
	mgr.sendMessage(addr, response.ToBytes())
}

func (mgr *SessionManager) handleJoinRoomRequest(addr *net.UDPAddr, data []byte) {
	request := msg.ParseJoinRoomRequest(data)
	if request == nil {
		logrus.Debugf("ParseJoinRoomRequest failed")
		return
	}
	var s *Session
	var exists bool
	if s, exists = mgr.roomToSessions[request.Room]; !exists {
		logrus.Debugf("Received JoinRoomRequest with invalid room id:%s", request.Room)
		return
	}
	s.SecondAddr = addr
	// TODO: 鉴权
	if _, exists = mgr.addrToSessions[addr.String()]; exists {
		logrus.Errorf("Received JoinRoomRequest(room:%s) from addrress(%s), but %s already exist for another session", request.Room, addr.String(), addr.String())
		return
	}
	mgr.addrToSessions[addr.String()] = s
	response := msg.NewJoinRoomResponse()
	mgr.sendMessage(addr, response.ToBytes())
}
