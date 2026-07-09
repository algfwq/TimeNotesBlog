package storage

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrAlreadyLiked  = errors.New("already liked")
	ErrInvalidInput  = errors.New("invalid input")
	ErrForbidden     = errors.New("forbidden")
)

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	CanUpload    bool   `json:"canUpload"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type Note struct {
	ID           string `json:"id"`
	OwnerUserID  string `json:"ownerUserId"`
	OwnerName    string `json:"ownerName,omitempty"`
	Filename     string `json:"filename"`
	Title        string `json:"title"`
	StoragePath  string `json:"-"`
	SizeBytes    int64  `json:"sizeBytes"`
	SHA256       string `json:"sha256"`
	Visible      bool   `json:"visible"`
	LikeCount    int64  `json:"likeCount"`
	CommentCount int64  `json:"commentCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	DownloadURL  string `json:"downloadUrl,omitempty"`
}

type Comment struct {
	ID        string `json:"id"`
	NoteID    string `json:"noteId"`
	Nickname  string `json:"nickname"`
	Email     string `json:"email"`
	GitHubURL string `json:"githubUrl"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

type GeoInfo struct {
	Country string  `json:"country"`
	Region  string  `json:"region"`
	City    string  `json:"city"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Source  string  `json:"source"`
}

type Visit struct {
	ID        string   `json:"id"`
	IPHash    string   `json:"ipHash"`
	Path      string   `json:"path"`
	NoteID    string   `json:"noteId"`
	Country   string   `json:"country"`
	Region    string   `json:"region"`
	City      string   `json:"city"`
	Lat       *float64 `json:"lat,omitempty"`
	Lng       *float64 `json:"lng,omitempty"`
	UserAgent string   `json:"userAgent"`
	CreatedAt string   `json:"createdAt"`
}

type VisitStats struct {
	TodayCount   int64            `json:"todayCount"`
	RecentCount  int64            `json:"recentCount"`
	Daily        []DailyCount     `json:"daily"`
	Locations    []VisitLocation  `json:"locations"`
	NoteStats    []NoteEngagement `json:"noteStats"`
}

type DailyCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type VisitLocation struct {
	Country string  `json:"country"`
	Region  string  `json:"region"`
	City    string  `json:"city"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Count   int64   `json:"count"`
}

type NoteEngagement struct {
	NoteID       string `json:"noteId"`
	Title        string `json:"title"`
	LikeCount    int64  `json:"likeCount"`
	CommentCount int64  `json:"commentCount"`
	Visible      bool   `json:"visible"`
}

type Store interface {
	Close() error

	EnsureAdmin(ctx context.Context, username, passwordHash string) (created bool, err error)
	CountUsers(ctx context.Context) (int64, error)
	CreateUser(ctx context.Context, user User) error
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	ListUsers(ctx context.Context) ([]User, error)
	UpdateUser(ctx context.Context, user User) error
	DeleteUser(ctx context.Context, id string) error
	UsernameExists(ctx context.Context, username string, excludeID string) (bool, error)

	CreateNote(ctx context.Context, note Note) error
	UpdateNoteFile(ctx context.Context, note Note) error
	GetNote(ctx context.Context, id string) (*Note, error)
	GetNoteByOwnerFilename(ctx context.Context, ownerID, filename string) (*Note, error)
	ListVisibleNotes(ctx context.Context) ([]Note, error)
	ListAllNotes(ctx context.Context) ([]Note, error)
	SetNoteVisible(ctx context.Context, id string, visible bool) error
	DeleteNote(ctx context.Context, id string) error

	AddLike(ctx context.Context, noteID, ipHash string) error
	HasLiked(ctx context.Context, noteID, ipHash string) (bool, error)

	AddComment(ctx context.Context, c Comment) error
	ListComments(ctx context.Context, noteID string) ([]Comment, error)

	GetLoginFailures(ctx context.Context, ipHash string) (count int, windowAt time.Time, err error)
	BumpLoginFailure(ctx context.Context, ipHash string, now time.Time) (count int, err error)
	ResetLoginFailures(ctx context.Context, ipHash string) error

	CreateDownloadToken(ctx context.Context, token, noteID string, expiresAt time.Time) error
	ConsumeDownloadToken(ctx context.Context, token string) (noteID string, err error)
	GetDownloadToken(ctx context.Context, token string) (noteID string, expiresAt time.Time, err error)

	GetGeoCache(ctx context.Context, ipHash string, maxAge time.Duration) (*GeoInfo, error)
	PutGeoCache(ctx context.Context, ipHash string, info GeoInfo) error

	AddVisit(ctx context.Context, v Visit) error
	GetVisitStats(ctx context.Context, recentDays int) (*VisitStats, error)
}
