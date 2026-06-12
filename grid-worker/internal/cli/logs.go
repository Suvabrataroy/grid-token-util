package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/grid-computing/grid-worker/pkg/platform"
)

// NewLogsCmd returns the `logs` subcommand.
func NewLogsCmd() *cobra.Command {
	var (
		follow bool
		lines  int
	)

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show daemon log output",
		Long:  "Display or follow the grid-worker daemon log file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(follow, lines)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output (like tail -f)")
	cmd.Flags().IntVarP(&lines, "lines", "n", 50, "number of lines to show from the end")

	return cmd
}

func runLogs(follow bool, lines int) error {
	logDir := platform.LogDir()
	logFile := filepath.Join(logDir, "grid-worker.log")

	// If a log file path is configured, use it
	if cfg != nil && cfg.Logging.File != "" {
		logFile = cfg.Logging.File
	}

	f, err := os.Open(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("log file not found at %q (is the daemon running?)", logFile)
		}
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	// Get file size for tail
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat log file: %w", err)
	}

	// Read last N lines
	if err := tailFile(f, info.Size(), lines); err != nil {
		return fmt.Errorf("tail log file: %w", err)
	}

	if !follow {
		return nil
	}

	// Follow mode: poll for new content
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	for {
		// Check for new content
		newInfo, err := f.Stat()
		if err != nil {
			return err
		}

		if newInfo.Size() > offset {
			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				return err
			}
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				fmt.Println(scanner.Text())
			}
			offset, _ = f.Seek(0, io.SeekCurrent)
		}

		time.Sleep(200 * time.Millisecond)
	}
}

// tailFile reads the last n lines from file f of totalSize bytes.
func tailFile(f *os.File, totalSize int64, n int) error {
	if totalSize == 0 {
		return nil
	}

	// Read the entire file if small, else scan backwards for newlines
	const maxScan = 1 << 20 // 1MB scan window
	scanSize := totalSize
	if scanSize > maxScan {
		scanSize = maxScan
	}

	buf := make([]byte, scanSize)
	if _, err := f.Seek(-scanSize, io.SeekEnd); err != nil {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
	}

	nr, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return err
	}
	buf = buf[:nr]

	// Find last N newlines
	var offsets []int
	for i := len(buf) - 1; i >= 0; i-- {
		if buf[i] == '\n' {
			offsets = append(offsets, i)
			if len(offsets) > n {
				break
			}
		}
	}

	start := 0
	if len(offsets) >= n {
		start = offsets[n-1] + 1
	}

	_, err = os.Stdout.Write(buf[start:])
	return err
}
