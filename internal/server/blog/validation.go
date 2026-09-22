package blog

import (
	"errors"
	"fmt"
	"strings"

	blogdata "thom-server/internal/blog"
)

const (
	maxTitleLength    = 200
	maxBodyLength     = 50000
	maxCategoryLength = 40
)

type postPatch struct {
	Title     *string `json:"title"`
	Body      *string `json:"body"`
	Category  *string `json:"category"`
	Published *bool   `json:"published"`
}

func normalizePost(post *blogdata.Post) {
	post.Title = strings.TrimSpace(post.Title)
	post.Body = strings.TrimSpace(post.Body)
	post.Category = strings.TrimSpace(post.Category)
}

func applyPostPatch(post *blogdata.Post, patch *postPatch) {
	if patch.Title != nil {
		post.Title = *patch.Title
	}
	if patch.Body != nil {
		post.Body = *patch.Body
	}
	if patch.Category != nil {
		post.Category = *patch.Category
	}
	if patch.Published != nil {
		post.Published = *patch.Published
	}
}

func isValidPost(post *blogdata.Post) error {
	if post.Title == "" {
		return errors.New("title is required")
	}
	if len([]rune(post.Title)) > maxTitleLength {
		return fmt.Errorf("title must be at most %d characters", maxTitleLength)
	}
	if len([]rune(post.Body)) > maxBodyLength {
		return fmt.Errorf("body must be at most %d characters", maxBodyLength)
	}
	if post.Category == "" {
		return errors.New("category is required")
	}
	if len([]rune(post.Category)) > maxCategoryLength {
		return fmt.Errorf("category must be at most %d characters", maxCategoryLength)
	}
	if blogdata.SlugifyName(post.Category) == "" {
		return errors.New("category must contain letters or numbers")
	}

	return nil
}
