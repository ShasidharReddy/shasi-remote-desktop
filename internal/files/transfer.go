package files

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ShasidharReddy/shasi-remote-desktop/internal/protocol"
)

type FileTransferManager struct {
	DownloadDir     string
	ActiveTransfers map[string]*Transfer
	mu              sync.Mutex
}

type Transfer struct {
	FileID   string
	FileName string
	FileSize int64
	File     *os.File
	Received int64
}

func NewFileTransferManager(downloadDir string) *FileTransferManager {
	os.MkdirAll(downloadDir, 0o755)
	return &FileTransferManager{
		DownloadDir:     downloadDir,
		ActiveTransfers: make(map[string]*Transfer),
	}
}

func (fm *FileTransferManager) StartTransfer(payload *protocol.FileTransferPayload) error {
	downloadPath := filepath.Join(fm.DownloadDir, filepath.Base(payload.FileName))

	file, err := os.Create(downloadPath)
	if err != nil {
		return fmt.Errorf("create file error: %w", err)
	}

	transfer := &Transfer{
		FileID:   payload.FileID,
		FileName: payload.FileName,
		FileSize: payload.FileSize,
		File:     file,
		Received: 0,
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if existing, exists := fm.ActiveTransfers[payload.FileID]; exists {
		if existing.File != nil {
			existing.File.Close()
			os.Remove(existing.File.Name())
		}
	}
	fm.ActiveTransfers[payload.FileID] = transfer
	log.Printf("Started file transfer: %s (ID: %s, Size: %d bytes)", payload.FileName, payload.FileID, payload.FileSize)
	return nil
}

func (fm *FileTransferManager) ReceiveChunk(payload *protocol.FileChunkPayload) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	transfer, exists := fm.ActiveTransfers[payload.FileID]
	if !exists {
		return fmt.Errorf("transfer not found: %s", payload.FileID)
	}

	if _, err := transfer.File.WriteAt(payload.Data, payload.Offset); err != nil {
		return fmt.Errorf("write error: %w", err)
	}

	transfer.Received += int64(len(payload.Data))
	if transfer.FileSize > 0 {
		percentage := (transfer.Received * 100) / transfer.FileSize
		if percentage%10 == 0 {
			log.Printf("File transfer progress: %s - %d%% (%d/%d bytes)",
				transfer.FileName, percentage, transfer.Received, transfer.FileSize)
		}
	}

	return nil
}

func (fm *FileTransferManager) EndTransfer(payload *protocol.FileEndPayload) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	transfer, exists := fm.ActiveTransfers[payload.FileID]
	if !exists {
		return fmt.Errorf("transfer not found: %s", payload.FileID)
	}

	defer func() {
		transfer.File.Close()
		delete(fm.ActiveTransfers, payload.FileID)
	}()

	if payload.Status == "success" {
		log.Printf("File transfer completed: %s", transfer.FileName)
		return nil
	}

	os.Remove(transfer.File.Name())
	return fmt.Errorf("transfer failed for %s", transfer.FileName)
}

func (fm *FileTransferManager) CleanupStaleTransfers() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	for fileID, transfer := range fm.ActiveTransfers {
		if transfer.File != nil {
			transfer.File.Close()
			os.Remove(transfer.File.Name())
		}
		delete(fm.ActiveTransfers, fileID)
		log.Printf("Cleaned up stale transfer: %s", transfer.FileName)
	}
}

func (fm *FileTransferManager) SendFile(filePath string) (*protocol.FileTransferPayload, []*protocol.FileChunkPayload, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open file error: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("stat error: %w", err)
	}

	chunkSize := 64 * 1024
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, fmt.Errorf("read error: %w", err)
	}

	fileID := fmt.Sprintf("%s_%d", stat.Name(), time.Now().UnixNano())
	transfer := &protocol.FileTransferPayload{
		FileName: stat.Name(),
		FileSize: stat.Size(),
		FileID:   fileID,
	}

	var chunks []*protocol.FileChunkPayload
	for offset := int64(0); offset < stat.Size(); offset += int64(chunkSize) {
		end := offset + int64(chunkSize)
		if end > stat.Size() {
			end = stat.Size()
		}

		chunk := &protocol.FileChunkPayload{
			FileID: fileID,
			Offset: offset,
			Data:   data[offset:end],
		}
		chunks = append(chunks, chunk)
	}

	log.Printf("Prepared file transfer: %s (%d chunks)", filePath, len(chunks))
	return transfer, chunks, nil
}
