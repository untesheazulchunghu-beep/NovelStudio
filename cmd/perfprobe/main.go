package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"novelstudio/internal/app"
)

type sample struct {
	Name              string  `json:"name"`
	Iterations        int     `json:"iterations"`
	CallsPerSample    int     `json:"calls_per_sample"`
	MedianMicros      float64 `json:"median_micros"`
	P95Micros         float64 `json:"p95_micros"`
	AverageAllocBytes int64   `json:"average_alloc_bytes"`
	AverageJSONBytes  int64   `json:"average_json_bytes"`
}

type report struct {
	GeneratedAt       string `json:"generated_at"`
	GoVersion         string `json:"go_version"`
	OS                string `json:"os"`
	Architecture      string `json:"architecture"`
	ChapterCount      int    `json:"chapter_count"`
	BytesPerChapter   int    `json:"bytes_per_chapter"`
	CorpusBytes       int64  `json:"corpus_bytes"`
	PostImportHeap    uint64 `json:"post_import_heap_bytes"`
	WorkspaceResponse sample `json:"workspace_response"`
	ReadChapter       sample `json:"read_chapter"`
	SearchCommon      sample `json:"search_common"`
	SearchRare        sample `json:"search_rare"`
	SearchShortHit    sample `json:"search_short_hit"`
	SearchShortMiss   sample `json:"search_short_miss"`
}

func main() {
	chapters := flag.Int("chapters", 100, "number of generated chapters (1-10000)")
	bytesPerChapter := flag.Int("bytes-per-chapter", 64<<10, "synthetic chapter body size in bytes")
	iterations := flag.Int("iterations", 20, "measured iterations per operation")
	flag.Parse()
	if *chapters < 1 || *chapters > 10000 {
		log.Fatal("-chapters must be between 1 and 10000")
	}
	if *bytesPerChapter < 32 || *bytesPerChapter > 16<<20 {
		log.Fatal("-bytes-per-chapter must be between 32 and 16777216")
	}
	if *iterations < 1 || *iterations > 10000 {
		log.Fatal("-iterations must be between 1 and 10000")
	}

	dataDir, err := os.MkdirTemp("", "novelstudio-perfprobe-")
	if err != nil {
		log.Fatalf("create temporary benchmark directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(dataDir); err != nil {
			log.Printf("remove temporary benchmark directory: %v", err)
		}
	}()

	store, err := app.NewStore(filepath.Join(dataDir, "benchmark.db"))
	if err != nil {
		log.Fatalf("open temporary database: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close temporary database: %v", err)
		}
	}()

	backup, err := store.CreateProjectBackup("default")
	if err != nil {
		log.Fatalf("create fixture template: %v", err)
	}
	backup.Project.Title = "Performance probe fixture"
	backup.Chapters = make([]app.Chapter, *chapters)
	body := syntheticBody(*bytesPerChapter)
	for index := range backup.Chapters {
		content := body
		if index == len(backup.Chapters)-1 {
			content = content[:len(content)-len("rareneedlexyz")] + "rareneedlexyz"
		}
		backup.Chapters[index] = app.Chapter{
			ID: int64(index + 1000), Number: index + 1,
			Title: fmt.Sprintf("Chapter %04d", index+1), Summary: "A long chapterphrase sample 灯火.",
			Content: content, Status: "draft",
		}
	}
	project, err := store.ImportProjectBackup(backup)
	if err != nil {
		log.Fatalf("import generated fixture: %v", err)
	}
	backup = app.ProjectBackup{}
	body = ""
	runtime.GC()
	var heap runtime.MemStats
	runtime.ReadMemStats(&heap)

	workspace, exists := store.MetadataSnapshot(project.WorkspaceID)
	if !exists {
		log.Fatal("read fixture metadata")
	}
	chapterID := workspace.Chapters[len(workspace.Chapters)-1].ID

	if err := verifySearchFixture(store, project.WorkspaceID, "chapterphrase", 15, "chapterphrase"); err != nil {
		log.Fatal(err)
	}
	if err := verifySearchFixture(store, project.WorkspaceID, "rareneedlexyz", 1, "rareneedlexyz"); err != nil {
		log.Fatal(err)
	}
	if err := verifySearchFixture(store, project.WorkspaceID, "灯火", 15, "灯火"); err != nil {
		log.Fatal(err)
	}
	if err := verifySearchFixture(store, project.WorkspaceID, "风", 0, ""); err != nil {
		log.Fatal(err)
	}

	result := report{
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		GoVersion:       runtime.Version(),
		OS:              runtime.GOOS,
		Architecture:    runtime.GOARCH,
		ChapterCount:    *chapters,
		BytesPerChapter: *bytesPerChapter,
		CorpusBytes:     int64(*chapters) * int64(*bytesPerChapter),
		PostImportHeap:  heap.HeapAlloc,
	}
	result.WorkspaceResponse = mustMeasure("workspace_response", *iterations, func() (int, error) {
		response, exists := store.Response(project.WorkspaceID)
		if !exists {
			return 0, errors.New("workspace response missing")
		}
		return encodedSize(response)
	})
	result.ReadChapter = mustMeasure("read_chapter", *iterations, func() (int, error) {
		chapter, exists, err := store.GetChapter(project.WorkspaceID, chapterID)
		if err != nil {
			return 0, err
		}
		if !exists {
			return 0, errors.New("chapter missing")
		}
		return encodedSize(chapter)
	})
	result.SearchCommon = mustMeasure("search_common", *iterations, func() (int, error) {
		chapters, exists, err := store.SearchChapters(project.WorkspaceID, "chapterphrase")
		if err != nil {
			return 0, err
		}
		if !exists {
			return 0, errors.New("workspace missing during search")
		}
		return encodedSize(chapters)
	})
	result.SearchRare = mustMeasure("search_rare", *iterations, func() (int, error) {
		chapters, exists, err := store.SearchChapters(project.WorkspaceID, "rareneedlexyz")
		if err != nil {
			return 0, err
		}
		if !exists {
			return 0, errors.New("workspace missing during search")
		}
		return encodedSize(chapters)
	})
	result.SearchShortHit = mustMeasure("search_short_hit", *iterations, func() (int, error) {
		chapters, exists, err := store.SearchChapters(project.WorkspaceID, "灯火")
		if err != nil {
			return 0, err
		}
		if !exists {
			return 0, errors.New("workspace missing during search")
		}
		return encodedSize(chapters)
	})
	result.SearchShortMiss = mustMeasure("search_short_miss", *iterations, func() (int, error) {
		chapters, exists, err := store.SearchChapters(project.WorkspaceID, "风")
		if err != nil {
			return 0, err
		}
		if !exists {
			return 0, errors.New("workspace missing during search")
		}
		return encodedSize(chapters)
	})

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		log.Fatalf("write benchmark report: %v", err)
	}
}

func verifySearchFixture(store *app.Store, workspaceID, query string, wantCount int, excerptText string) error {
	results, exists, err := store.SearchChapters(workspaceID, query)
	if err != nil {
		return fmt.Errorf("verify search fixture %q: %w", query, err)
	}
	if !exists || len(results) != wantCount {
		return fmt.Errorf("verify search fixture %q: got %d results (workspace found: %t), want %d", query, len(results), exists, wantCount)
	}
	if excerptText != "" {
		excerpt := ""
		if len(results) > 0 {
			excerpt = results[0].Excerpt
		}
		if !strings.Contains(strings.ToLower(excerpt), strings.ToLower(excerptText)) {
			return fmt.Errorf("verify search fixture %q: result excerpt %q did not contain the match", query, excerpt)
		}
	}
	return nil
}

func syntheticBody(size int) string {
	const paragraph = "The lantern crossed the quiet harbor. The old clock answered after midnight. "
	return strings.Repeat(paragraph, size/len(paragraph)+1)[:size]
}

func encodedSize(value any) (int, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0, err
	}
	return len(encoded), nil
}

func mustMeasure(name string, iterations int, run func() (int, error)) sample {
	if _, err := run(); err != nil {
		log.Fatalf("warm up %s: %v", name, err)
	}
	runtime.GC()
	const callsPerSample = 10
	times := make([]time.Duration, iterations)
	var totalAlloc, totalJSON int64
	for index := 0; index < iterations; index++ {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		started := time.Now()
		for call := 0; call < callsPerSample; call++ {
			jsonBytes, err := run()
			if err != nil {
				log.Fatalf("measure %s: %v", name, err)
			}
			totalJSON += int64(jsonBytes)
		}
		times[index] = time.Since(started) / callsPerSample
		runtime.ReadMemStats(&after)
		totalAlloc += int64(after.TotalAlloc - before.TotalAlloc)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	p95Index := (len(times)*95+99)/100 - 1
	return sample{
		Name: name, Iterations: iterations, CallsPerSample: callsPerSample,
		MedianMicros:      float64(times[len(times)/2]) / float64(time.Microsecond),
		P95Micros:         float64(times[p95Index]) / float64(time.Microsecond),
		AverageAllocBytes: totalAlloc / int64(iterations*callsPerSample),
		AverageJSONBytes:  totalJSON / int64(iterations*callsPerSample),
	}
}
