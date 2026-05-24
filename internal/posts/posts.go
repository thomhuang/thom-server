package posts

import (
	"database/sql"
	"errors"

	"thom-server/internal/data"
)

type Category struct {
	ID       int
	Category string
}

type Post struct {
	ID         int
	CategoryID int
	Title      string
	Summary    string
	PathName   string
	Content    []Content
	Link       string
}

type Content struct {
	ID        int
	PostID    int
	Text      string
	ImagePath string
}

type Model struct {
	DB *sql.DB
}

func (m *Model) GetCategories() ([]*Category, error) {
	stmt := `
		SELECT id, Category
		FROM PostCategories
		ORDER BY id`

	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := make([]*Category, 0)
	for rows.Next() {
		category := &Category{}
		if err = rows.Scan(&category.ID, &category.Category); err != nil {
			return nil, err
		}

		categories = append(categories, category)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return categories, nil
}

func (m *Model) GetPostsByCategory(categoryID int) ([]*Post, error) {
	stmt := `
		SELECT id, CategoryID, Title, Summary, PathName, Link
		FROM Posts
		WHERE CategoryId = ?
		ORDER BY id`

	rows, err := m.DB.Query(stmt, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := make([]*Post, 0)
	for rows.Next() {
		post, err := scanPost(rows)
		if err != nil {
			return nil, err
		}

		posts = append(posts, post)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return posts, nil
}

func (m *Model) GetPostWithContentByID(id int) (*Post, error) {
	stmt := `
		SELECT id, CategoryID, Title, Summary, PathName, Link
		FROM Posts
		WHERE id = ?`

	return m.getPostWithContent(stmt, id)
}

func (m *Model) GetPostWithContentByPathName(pathName string) (*Post, error) {
	stmt := `
		SELECT id, CategoryID, Title, Summary, PathName, Link
		FROM Posts
		WHERE PathName = ?`

	return m.getPostWithContent(stmt, pathName)
}

func (m *Model) getPostWithContent(stmt string, args ...any) (*Post, error) {
	post, err := scanPost(m.DB.QueryRow(stmt, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, data.ErrNoRecord
		}
		return nil, err
	}

	content, err := m.getPostContent(post.ID)
	if err != nil {
		return nil, err
	}
	post.Content = content

	return post, nil
}

func (m *Model) getPostContent(postID int) ([]Content, error) {
	stmt := `
		SELECT id, PostID, Text, ImagePath
		FROM PostContent
		WHERE PostId = ?
		ORDER BY id`

	rows, err := m.DB.Query(stmt, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	content := make([]Content, 0)
	for rows.Next() {
		contentChunk, err := scanPostContent(rows)
		if err != nil {
			return nil, err
		}

		content = append(content, contentChunk)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return content, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPost(s scanner) (*Post, error) {
	post := &Post{}
	var link sql.NullString

	err := s.Scan(&post.ID, &post.CategoryID, &post.Title, &post.Summary, &post.PathName, &link)
	if err != nil {
		return nil, err
	}
	post.Link = link.String

	return post, nil
}

func scanPostContent(s scanner) (Content, error) {
	var content Content
	var text, imagePath sql.NullString

	err := s.Scan(&content.ID, &content.PostID, &text, &imagePath)
	if err != nil {
		return Content{}, err
	}
	content.Text = text.String
	content.ImagePath = imagePath.String

	return content, nil
}
