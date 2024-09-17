package models

import (
	"database/sql"
	"errors"
)

type PostCategory struct {
	ID       int
	Category string
}

type Post struct {
	ID         int
	CategoryID int
	Title      string
	Summary    string
	PathName   string
	Content    []PostContent
	Link       sql.NullString
}

type PostContent struct {
	ID        int
	PostId    int
	Text      sql.NullString
	ImagePath sql.NullString
}

type PostModel struct {
	DB *sql.DB
}

func (m *PostModel) GetCategories() ([]*PostCategory, error) {
	stmt := `SELECT id, Category FROM PostCategories`

	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var categories []*PostCategory
	for rows.Next() {
		currCategory := &PostCategory{}

		err = rows.Scan(&currCategory.ID, &currCategory.Category)
		if err != nil {
			return nil, err
		}

		categories = append(categories, currCategory)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return categories, nil
}

func (m *PostModel) GetPosts(id int) (*Post, error) {
	stmt := `SELECT id, CategoryId, Title, Summary, PathName, Link FROM Posts WHERE CategoryId = ? ORDER BY id`

	post := &Post{}

	err := m.DB.QueryRow(stmt, id).Scan(&post.ID, &post.CategoryID, &post.Title, &post.PathName, &post.Summary, &post.Link)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		} else {
			return nil, err
		}

	}

	return post, nil
}
