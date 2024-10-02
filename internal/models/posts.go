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
	Link       string
}

type PostContent struct {
	ID        int
	PostID    int
	Text      string
	ImagePath string
}

type PostModel struct {
	DB *sql.DB
}

func (m *PostModel) GetCategories() ([]*PostCategory, error) {
	stmt := `SELECT * FROM PostCategories`

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
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrNoRecord
			} else {
				return nil, err
			}
		}

		categories = append(categories, currCategory)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return categories, nil
}

func (m *PostModel) GetPosts(id int) ([]*Post, error) {
	stmt := `
		SELECT id, CategoryID, Title, Summary, PathName, Link
		FROM Posts
		WHERE CategoryId = ?
		ORDER BY id`

	rows, err := m.DB.Query(stmt, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*Post
	for rows.Next() {
		currPost := &Post{}

		err = rows.Scan(&currPost.ID, &currPost.CategoryID, &currPost.Title, &currPost.Summary, &currPost.PathName, &currPost.Link)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrNoRecord
			} else {
				return nil, err
			}
		}

		posts = append(posts, currPost)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return posts, nil
}

func (m *PostModel) GetPostById(id int) (*Post, error) {
	stmt := `
		SELECT id, Title, PathName, Link
		FROM Posts
		WHERE id = ?`

	post := &Post{}
	err := m.DB.QueryRow(stmt, id).Scan(&post.ID, &post.Title, &post.PathName, &post.Link)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		} else {
			return nil, err
		}
	}

	return post, nil
}

func (m *PostModel) GetPostContentById(id int) ([]*PostContent, error) {
	stmt := `
		SELECT id, PostID, Text, ImagePath
		FROM PostContent
		WHERE PostId = ?
		ORDER BY id`

	rows, err := m.DB.Query(stmt, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postContent []*PostContent
	for rows.Next() {
		content := &PostContent{}

		err = rows.Scan(&content.ID, &content.PostID, &content.Text, &content.ImagePath)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrNoRecord
			} else {
				return nil, err
			}
		}

		postContent = append(postContent, content)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return postContent, nil
}

func (m *PostModel) GetPostByPathName(pathName string) (*Post, error) {
	stmt := `
		SELECT id, Title, PathName, Link
		FROM Posts
		WHERE PathName = ?`

	post := &Post{}
	err := m.DB.QueryRow(stmt, pathName).Scan(&post.ID, &post.Title, &post.PathName, &post.Link)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		} else {
			return nil, err
		}
	}

	return post, nil
}

func (m *PostModel) GetPostContentByPathName(pathName string) ([]*PostContent, error) {
	stmt := `
		SELECT PostContent.id, PostContent.PostID, PostContent.Text, PostContent.ImagePath
		From PostContent JOIN Posts ON PostContent.PostId = Posts.Id
		WHERE Posts.PathName = ?`

	rows, err := m.DB.Query(stmt, pathName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postContent []*PostContent
	for rows.Next() {
		content := &PostContent{}

		err = rows.Scan(&content.ID, &content.PostID, &content.Text, &content.ImagePath)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrNoRecord
			} else {
				return nil, err
			}
		}

		postContent = append(postContent, content)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return postContent, nil
}
