package telegram

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

const (
	MaxMediaSize = 5 * 1024 * 1024 // 5MB
)

// Media handles media downloads and uploads
type Media struct {
	bot    *Bot
	logger zerolog.Logger
}

// MediaFile represents a downloaded media file
type MediaFile struct {
	FileID   string
	FilePath string
	FileSize int
	MimeType string
	Type     string // photo, video, audio, document, voice
}

// NewMedia creates a new media handler
func NewMedia(bot *Bot) *Media {
	return &Media{
		bot:    bot,
		logger: bot.logger.With().Str("module", "media").Logger(),
	}
}

// HandleMedia processes media messages
func (m *Media) HandleMedia(update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}

	msg := update.Message

	// Determine media type and file ID
	var fileID string
	var mediaType string

	if len(msg.Photo) > 0 {
		fileID = msg.Photo[len(msg.Photo)-1].FileID
		mediaType = "photo"
	} else if msg.Video != nil {
		fileID = msg.Video.FileID
		mediaType = "video"
	} else if msg.Audio != nil {
		fileID = msg.Audio.FileID
		mediaType = "audio"
	} else if msg.Document != nil {
		fileID = msg.Document.FileID
		mediaType = "document"
	} else if msg.Voice != nil {
		fileID = msg.Voice.FileID
		mediaType = "voice"
	} else {
		return nil
	}

	m.logger.Debug().
		Str("file_id", fileID).
		Str("type", mediaType).
		Int64("chat_id", msg.Chat.ID).
		Msg("Media received")

	return nil
}

// DownloadFile downloads a file from Telegram
func (m *Media) DownloadFile(fileID string, destPath string) (*MediaFile, error) {
	// Get file info
	file, err := m.bot.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Check file size
	if file.FileSize > MaxMediaSize {
		return nil, fmt.Errorf("file size %d exceeds maximum %d", file.FileSize, MaxMediaSize)
	}

	// Get download URL
	url := file.Link(m.bot.api.Token)

	// Download file
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create destination directory
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Copy data
	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	m.logger.Info().
		Str("file_id", fileID).
		Str("path", destPath).
		Int64("size", written).
		Msg("File downloaded")

	return &MediaFile{
		FileID:   fileID,
		FilePath: destPath,
		FileSize: int(written),
		Type:     "unknown",
	}, nil
}

// DownloadPhoto downloads a photo
func (m *Media) DownloadPhoto(fileID string, destPath string) (*MediaFile, error) {
	media, err := m.DownloadFile(fileID, destPath)
	if err != nil {
		return nil, err
	}
	media.Type = "photo"
	return media, nil
}

// DownloadVideo downloads a video
func (m *Media) DownloadVideo(fileID string, destPath string) (*MediaFile, error) {
	media, err := m.DownloadFile(fileID, destPath)
	if err != nil {
		return nil, err
	}
	media.Type = "video"
	return media, nil
}

// DownloadAudio downloads audio
func (m *Media) DownloadAudio(fileID string, destPath string) (*MediaFile, error) {
	media, err := m.DownloadFile(fileID, destPath)
	if err != nil {
		return nil, err
	}
	media.Type = "audio"
	return media, nil
}

// DownloadDocument downloads a document
func (m *Media) DownloadDocument(fileID string, destPath string) (*MediaFile, error) {
	media, err := m.DownloadFile(fileID, destPath)
	if err != nil {
		return nil, err
	}
	media.Type = "document"
	return media, nil
}

// UploadPhoto uploads a photo with caption
func (m *Media) UploadPhoto(chatID int64, photoPath string, caption string) error {
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(photoPath))
	photo.Caption = caption

	_, err := m.bot.api.Send(photo)
	if err != nil {
		return fmt.Errorf("failed to upload photo: %w", err)
	}

	m.logger.Info().
		Int64("chat_id", chatID).
		Str("path", photoPath).
		Msg("Photo uploaded")

	return nil
}

// UploadDocument uploads a document
func (m *Media) UploadDocument(chatID int64, docPath string, caption string) error {
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(docPath))
	doc.Caption = caption

	_, err := m.bot.api.Send(doc)
	if err != nil {
		return fmt.Errorf("failed to upload document: %w", err)
	}

	m.logger.Info().
		Int64("chat_id", chatID).
		Str("path", docPath).
		Msg("Document uploaded")

	return nil
}

// GetFileSize returns the size of a file
func (m *Media) GetFileSize(fileID string) (int, error) {
	file, err := m.bot.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return 0, fmt.Errorf("failed to get file info: %w", err)
	}
	return file.FileSize, nil
}
