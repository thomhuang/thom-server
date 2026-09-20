package shop

import (
	"strconv"

	"thom-server/internal/data"
)

func (m *Model) AddImage(itemID int, image *Image) (*Image, error) {
	stmt := `
		INSERT INTO ShopItemImages (ItemID, ObjectKey, AltText, SortOrder)
		VALUES (?, ?, ?, ?)`

	result, err := m.DB.Exec(stmt, itemID, image.ObjectKey, image.AltText, image.SortOrder)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	image.ID = strconv.FormatInt(id, 10)

	return image, nil
}

// DeleteImage detaches an image from a listing and returns it so the caller can
// delete the R2 object.
func (m *Model) DeleteImage(itemID, imageID int) (*Image, error) {
	stmt := `
		SELECT id, ObjectKey, AltText, SortOrder
		FROM ShopItemImages
		WHERE id = ? AND ItemID = ?`

	image := &Image{}
	var id int

	err := m.DB.QueryRow(stmt, imageID, itemID).Scan(
		&id,
		&image.ObjectKey,
		&image.AltText,
		&image.SortOrder,
	)
	if err != nil {
		return nil, data.NoRecord(err)
	}
	image.ID = strconv.Itoa(id)

	if _, err = m.DB.Exec(`DELETE FROM ShopItemImages WHERE id = ?`, imageID); err != nil {
		return nil, err
	}

	return image, nil
}

func (m *Model) CountImages(itemID int) (int, error) {
	var count int
	err := m.DB.QueryRow(
		`SELECT COUNT(*) FROM ShopItemImages WHERE ItemID = ?`, itemID,
	).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// ItemImageCount reports whether an item exists and how many images it has, in
// a single round trip.
func (m *Model) ItemImageCount(itemID int) (bool, int, error) {
	var exists, count int
	err := m.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM ShopItems WHERE id = ?),
			(SELECT COUNT(*) FROM ShopItemImages WHERE ItemID = ?)`,
		itemID,
		itemID,
	).Scan(&exists, &count)
	if err != nil {
		return false, 0, err
	}

	return exists != 0, count, nil
}

func (m *Model) getImages(itemID int) ([]*Image, error) {
	stmt := `
		SELECT id, ObjectKey, AltText, SortOrder
		FROM ShopItemImages
		WHERE ItemID = ?
		ORDER BY SortOrder ASC, id ASC`

	rows, err := m.DB.Query(stmt, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	images := make([]*Image, 0)
	for rows.Next() {
		image := &Image{}
		var id int

		if err = rows.Scan(&id, &image.ObjectKey, &image.AltText, &image.SortOrder); err != nil {
			return nil, err
		}

		image.ID = strconv.Itoa(id)
		images = append(images, image)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return images, nil
}
