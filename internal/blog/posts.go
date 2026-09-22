package blog

import (
	"database/sql"
	"strconv"

	"thom-server/internal/data"
)

// postColumns is the full column list every post read scans, kept in one place
// so list and single-post reads cannot drift apart. Posts join their category
// row, so the category name never needs a follow-up request.
const postColumns = `p.id, p.Title, p.Body, p.CategoryID, c.Name, p.Published, p.CreatedAt, p.UpdatedAt`

// GetAll returns every post, newest first. publishedOnly keeps drafts out of
// the public list; a non-empty categoryID restricts the list to one category.
func (m *Model) GetAll(publishedOnly bool, categoryID string) ([]*Post, error) {
	stmt := `SELECT ` + postColumns + `
		FROM BlogPosts p
		JOIN BlogCategories c ON c.id = p.CategoryID`
	if categoryID != "" {
		stmt += `
		WHERE p.CategoryID = ?`
		if publishedOnly {
			stmt += `
		AND p.Published = 1`
		}
	} else if publishedOnly {
		stmt += `
		WHERE p.Published = 1`
	}
	stmt += `
		ORDER BY p.id DESC`

	var (
		rows *sql.Rows
		err  error
	)
	if categoryID != "" {
		rows, err = m.DB.Query(stmt, categoryID)
	} else {
		rows, err = m.DB.Query(stmt)
	}
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

func (m *Model) GetByID(id int) (*Post, error) {
	stmt := `SELECT ` + postColumns + `
		FROM BlogPosts p
		JOIN BlogCategories c ON c.id = p.CategoryID
		WHERE p.id = ?`

	post, err := scanPost(m.DB.QueryRow(stmt, id))
	if err != nil {
		return nil, data.NoRecord(err)
	}

	return post, nil
}

func (m *Model) Insert(post *Post) (*Post, error) {
	category, err := m.upsertCategory(post.Category)
	if err != nil {
		return nil, err
	}

	stmt := `
		INSERT INTO BlogPosts (Title, Body, CategoryID, Published)
		VALUES (?, ?, ?, ?)`

	result, err := m.DB.Exec(stmt, post.Title, post.Body, category.ID, post.Published)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return m.GetByID(int(id))
}

func (m *Model) Update(id int, post *Post) (*Post, error) {
	category, err := m.upsertCategory(post.Category)
	if err != nil {
		return nil, err
	}

	stmt := `
		UPDATE BlogPosts
		SET Title = ?,
			Body = ?,
			CategoryID = ?,
			Published = ?,
			UpdatedAt = datetime('now')
		WHERE id = ?`

	result, err := m.DB.Exec(stmt, post.Title, post.Body, category.ID, post.Published, id)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, data.ErrNoRecord
	}

	return m.GetByID(id)
}

func (m *Model) Delete(id int) error {
	stmt := `
		DELETE FROM BlogPosts
		WHERE id = ?`

	result, err := m.DB.Exec(stmt, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return data.ErrNoRecord
	}

	return nil
}

func scanPost(s scanner) (*Post, error) {
	post := &Post{}
	var id int
	var published int

	err := s.Scan(&id, &post.Title, &post.Body, &post.CategoryID, &post.Category, &published, &post.CreatedAt, &post.UpdatedAt)
	if err != nil {
		return nil, err
	}

	post.ID = strconv.Itoa(id)
	post.Published = published == 1

	return post, nil
}
