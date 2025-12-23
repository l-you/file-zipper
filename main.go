package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

const OUTPUT_DIR = "/output"
const SOURCE_DIR = "/storage"

type zipFileName struct {
	Name  string `json:"name"`
	Ext   string `json:"ext"`
	Alias string `json:"alias"`
}
type zipRequest struct {
	Filenames []zipFileName `json:"filenames"`
}

type zipResponse struct {
	FileID string `json:"file_id"`
}

func logSkipped(i string, err error) {

	fmt.Printf("File skipped %s %s\n", i, err.Error())
}
func logErr(err error) {

	fmt.Printf("Unexpected error %s\n", err.Error())
}
func copyFile(source string, destPath string) error {
	fmt.Printf("Copy from %s to %s", source, destPath)

	input, err := os.ReadFile(source)
	if err != nil {
		fmt.Println("Read failed", err)
		return err
	}

	err = os.WriteFile(destPath, input, 0644)
	if err != nil {
		fmt.Println("Error creating", destPath)
		return err
	}
	return nil
}
func writeFileToZip(w *zip.Writer, e zipFileName) error {
	spath := fmt.Sprintf(".%s/%s", SOURCE_DIR, e.Name)
	file, err := os.Open(spath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	header.Name = filepath.ToSlash(fmt.Sprintf("%s.%s", e.Alias, e.Ext))
	header.Method = zip.Store

	// Set the UTF-8 flag for filenames to ensure correct encoding
	header.Flags |= 0x800

	writer, err := w.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	if err != nil {
		return err
	}
	fmt.Println("Added file ", spath)
	return nil
}

func createZipArchive(fnms []zipFileName) (string, error) {
	tmpF, err := os.CreateTemp("", "tmp-zip-*")
	fmt.Println("Tmp file path", tmpF.Name())
	if err != nil {
		return "", err
	}
	defer tmpF.Close()
	zipWriter := zip.NewWriter(tmpF)
	defer zipWriter.Close()
	for _, e := range fnms {
		err := writeFileToZip(zipWriter, e)
		if nil != err {
			logSkipped(e.Name, err)
			continue
		}
	}
	return tmpF.Name(), nil
}
func cleanOldFiles(dir string, olderThan time.Duration) {
	now := time.Now()
    log.Printf("Cleaning old files")

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if the file is older than the specified duration
		if now.Sub(info.ModTime()) > olderThan {
			log.Printf("Deleting file: %s, Last modified: %v\n", path, info.ModTime())
			if err := os.Remove(path); err != nil {
				log.Printf("Failed to delete file: %s, error: %v\n", path, err)
			}
		}

		return nil
	})

	if err != nil {
		log.Printf("Error walking the directory: %v\n", err)
	}
}
func runCron() {
	// create a scheduler
	s, err := gocron.NewScheduler()
	if err != nil {
		log.Fatal(err)
		return
	}

	// add a job to the scheduler
	j, err := s.NewJob(
		gocron.DurationJob(
			24*time.Hour,
		),
		gocron.NewTask(
			func(a string, b int) {
				fmt.Println("Running crontab")
				cleanOldFiles("/app/output", time.Hour*24*7)
			},
			"hello",
			1,
		),
	)
	if err != nil {
		log.Fatal(err)
		return
	}
	// each job has a unique id
	fmt.Println(j.ID())

	// start the scheduler
	s.Start()


}
func handleZip(id string, fnms []zipFileName) error {
	start := time.Now()
	tmpF, err := createZipArchive(fnms)
	defer os.Remove(tmpF)
	if err != nil {
		logErr(err)
		return err
	}

	destFName := fmt.Sprintf(".%s/%s.zip", OUTPUT_DIR, id)
	if err := copyFile(tmpF, destFName); err != nil {
		logErr(err)
		return err
	}
	elapsed := time.Since(start)
	fmt.Println("Zip execution time: ", elapsed)
	return nil
}
func zipFilesHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Request %s %s %s\n", r.Method, r.URL.Path, r.Host)
	switch r.Method {
	case http.MethodPost:
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req zipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		logErr(err)
		return
	}
	fileID, err := uuid.NewUUID()
	fmt.Printf("File id %s\n", fileID)
	if err != nil {
		logErr(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := fileID.String()
	go handleZip(id, req.Filenames)

	resp := zipResponse{FileID: id}
	fmt.Printf("Successfully generated %s\n", fileID)
	json.NewEncoder(w).Encode(resp)
}
type clearResp struct {
	status string `json:"status"`
}
func cleanOldFilesManual(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Request %s %s %s\n", r.Method, r.URL.Path, r.Host)
    cleanOldFiles("/app/output", time.Hour*24*7)
	resp := clearResp{status: "ok"}
	json.NewEncoder(w).Encode(resp)
}

func main() {
	runCron()
	http.HandleFunc("/zip", zipFilesHandler)
	http.HandleFunc("/clean-old", cleanOldFilesManual)

	fmt.Println("Server started at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
