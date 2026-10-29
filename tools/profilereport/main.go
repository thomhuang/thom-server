// Command profilereport lists the distinct pour recipes across existing coffee
// entries so they can be reviewed before migrating them into brew profiles.
//
// It is strictly read-only: it fetches entries from the coffee API and prints a
// report. It never writes to the database.
//
// Usage:
//
//	go run ./tools/profilereport -base http://localhost:4000
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// entry carries only the fields the report needs, so the tool does not depend
// on the coffee package's full model.
type entry struct {
	ID         string `json:"id"`
	Date       string `json:"date"`
	CoffeeName string `json:"coffeeName"`
	BrewMethod string `json:"brewMethod"`
	PourNotes  string `json:"pourNotes"`
}

type bucket struct {
	method  string
	notes   string // first-seen original text, for display
	entries []entry
}

func main() {
	base := flag.String("base", "http://localhost:4000", "API base URL")
	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(strings.TrimRight(*base, "/") + "/coffee")
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch entries:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		fmt.Fprintf(os.Stderr, "GET /coffee returned %s: %s\n", resp.Status, body)
		os.Exit(1)
	}

	var entries []entry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		fmt.Fprintln(os.Stderr, "decode entries:", err)
		os.Exit(1)
	}

	// Buckets are keyed by brew method + normalized recipe text, so identical
	// recipes (ignoring case and extra whitespace) group together.
	byKey := map[string]*bucket{}
	var withNotes int
	for _, e := range entries {
		norm := normalize(e.PourNotes)
		if norm == "" {
			continue
		}
		withNotes++
		key := e.BrewMethod + "\x00" + norm
		b, ok := byKey[key]
		if !ok {
			b = &bucket{method: e.BrewMethod, notes: e.PourNotes}
			byKey[key] = b
		}
		b.entries = append(b.entries, e)
	}

	buckets := make([]*bucket, 0, len(byKey))
	for _, b := range byKey {
		buckets = append(buckets, b)
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].method != buckets[j].method {
			return buckets[i].method < buckets[j].method
		}
		return len(buckets[i].entries) > len(buckets[j].entries)
	})

	fmt.Printf("%d entries total, %d with pour notes, %d distinct recipes\n",
		len(entries), withNotes, len(buckets))
	fmt.Println("(matching is exact after case and whitespace are ignored; near-duplicates are listed separately)")
	fmt.Println()

	for _, b := range buckets {
		if len(b.entries) == 1 {
			fmt.Printf("[%s] 1 brew (singleton)\n", b.method)
		} else {
			fmt.Printf("[%s] %d brews\n", b.method, len(b.entries))
		}
		for _, e := range b.entries {
			fmt.Printf("    #%s %s %s\n", e.ID, e.Date, e.CoffeeName)
		}
		fmt.Printf("    recipe: %s\n\n", indent(b.notes))
	}
}

// normalize lowercases and collapses whitespace so matching is lenient about
// formatting but not about wording.
func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// indent puts the first line inline and aligns any following lines under it.
func indent(s string) string {
	return strings.ReplaceAll(s, "\n", "\n            ")
}
