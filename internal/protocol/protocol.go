package protocol

import (
	"encoding/json"
	"time"
)

const Version = 1

const (
	TypeError = "error"

	TypeAuthPowChallenge = "auth.pow.challenge"
	TypeAuthLogin        = "auth.login"
	TypeAuthPing         = "auth.ping"
	TypeAuthSession      = "auth.session"

	TypeNotesList         = "notes.list"
	TypeNotesGet          = "notes.get"
	TypeNotesUploadStart  = "notes.upload.start"
	TypeNotesUploadChunk  = "notes.upload.chunk"
	TypeNotesUploadFinish = "notes.upload.finish"
	TypeNotesUpdateStart  = "notes.update.start"
	TypeNotesUpdateChunk  = "notes.update.chunk"
	TypeNotesUpdateFinish = "notes.update.finish"
	TypeNotesLike         = "notes.like"
	TypeNotesCommentsList = "notes.comments.list"
	TypeNotesCommentCreate = "notes.comment.create"

	TypeVisitTrack = "visit.track"

	TypeAdminNotesList     = "admin.notes.list"
	TypeAdminNoteSetVisible = "admin.notes.set_visible"
	TypeAdminNoteDelete    = "admin.notes.delete"
	TypeAdminNoteUploadStart = "admin.notes.upload.start"
	TypeAdminNoteUploadChunk = "admin.notes.upload.chunk"
	TypeAdminNoteUploadFinish = "admin.notes.upload.finish"
	TypeAdminUsersList     = "admin.users.list"
	TypeAdminUserCreate    = "admin.users.create"
	TypeAdminUserDelete    = "admin.users.delete"
	TypeAdminUserUpdate    = "admin.users.update"
	TypeAdminStats         = "admin.stats"
	TypeAdminSelfUpdate    = "admin.self.update"
)

type Envelope struct {
	Version int             `json:"v"`
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *ErrorPayload   `json:"error,omitempty"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewEnvelope(msgType, id string, payload any) (Envelope, error) {
	env := Envelope{Version: Version, Type: msgType, ID: id}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return env, err
		}
		env.Payload = raw
	}
	return env, nil
}

func NewError(id, code, message string) Envelope {
	return Envelope{
		Version: Version,
		Type:    TypeError,
		ID:      id,
		Error:   &ErrorPayload{Code: code, Message: message},
	}
}

func DecodePayload[T any](env Envelope) (T, error) {
	var out T
	if len(env.Payload) == 0 {
		return out, nil
	}
	err := json.Unmarshal(env.Payload, &out)
	return out, err
}

func NowUnix() int64 {
	return time.Now().Unix()
}
