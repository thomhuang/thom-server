package coffee

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	coffeedata "thom-server/internal/coffee"
	"thom-server/internal/server/response"
)

type Handler struct {
	coffee    *coffeedata.Model
	responder response.Responder
}

func New(model *coffeedata.Model, responder response.Responder) *Handler {
	return &Handler{
		coffee:    model,
		responder: responder,
	}
}

type coffeeEntryPatch struct {
	Date             *string `json:"date"`
	CoffeeName       *string `json:"coffeeName"`
	Origin           *string `json:"origin"`
	CoffeeVarietal   *string `json:"coffeeVarietal"`
	ProcessingMethod *string `json:"processingMethod"`
	DaysSinceRoast   *string `json:"daysSinceRoast"`
	RoasterID        *string `json:"roasterId"`
	Roaster          *string `json:"roaster"`
	BrewMethod       *string `json:"brewMethod"`
	Ratio            *string `json:"ratio"`
	Grinder          *string `json:"grinder"`
	GrindSetting     *string `json:"grindSetting"`
	Dose             *string `json:"dose"`
	YieldAmount      *string `json:"yieldAmount"`
	WaterTemperature *string `json:"waterTemperature"`
	BrewTime         *string `json:"brewTime"`
	BloomTime        *string `json:"bloomTime"`
	BloomWater       *string `json:"bloomWater"`
	PourNotes        *string `json:"pourNotes"`
	RoastLevel       *string `json:"roastLevel"`
	Notes            *string `json:"notes"`
	TastingNotes     *string `json:"tastingNotes"`
	Rating           *int    `json:"rating"`
}

func (h *Handler) GetEntries(w http.ResponseWriter, r *http.Request) {
	entries, err := h.coffee.GetAll()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, entries, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) GetEntryByID(w http.ResponseWriter, r *http.Request) {
	id, ok := h.readPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	entry, err := h.coffee.GetByID(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, entry, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) GetRoasters(w http.ResponseWriter, r *http.Request) {
	roasters, err := h.coffee.GetRoasters()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, roasters, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) CreateRoaster(w http.ResponseWriter, r *http.Request) {
	var roaster coffeedata.Roaster
	if err := json.NewDecoder(r.Body).Decode(&roaster); err != nil {
		h.responder.BadRequest(w)
		return
	}

	normalizeCoffeeRoaster(&roaster)
	if !isValidCoffeeRoaster(&roaster) {
		h.responder.BadRequest(w)
		return
	}

	createdRoaster, err := h.coffee.UpsertRoaster(&roaster)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusCreated, createdRoaster, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	var entry coffeedata.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		h.responder.BadRequest(w)
		return
	}

	normalizeCoffeeEntry(&entry)
	if !isValidCoffeeEntry(&entry) {
		h.responder.BadRequest(w)
		return
	}

	createdEntry, err := h.coffee.Insert(&entry)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusCreated, createdEntry, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) UpdateEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := h.readPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	var patch coffeeEntryPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		h.responder.BadRequest(w)
		return
	}
	if (patch.RoasterID == nil) != (patch.Roaster == nil) {
		h.responder.BadRequest(w)
		return
	}

	entry, err := h.coffee.GetByID(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	applyCoffeeEntryPatch(entry, &patch)
	normalizeCoffeeEntry(entry)
	if !isValidCoffeeEntry(entry) {
		h.responder.BadRequest(w)
		return
	}

	updatedEntry, err := h.coffee.Update(id, entry)
	if h.responder.HandleDataError(w, err) {
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, updatedEntry, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := h.readPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	err := h.coffee.Delete(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) readPositiveIntPath(w http.ResponseWriter, r *http.Request, key string) (int, bool) {
	id, err := strconv.Atoi(r.PathValue(key))
	if err != nil || id < 1 {
		h.responder.BadRequest(w)
		return 0, false
	}

	return id, true
}

func normalizeCoffeeEntry(entry *coffeedata.Entry) {
	entry.Date = strings.TrimSpace(entry.Date)
	entry.CoffeeName = strings.TrimSpace(entry.CoffeeName)
	entry.Origin = strings.TrimSpace(entry.Origin)
	entry.CoffeeVarietal = strings.TrimSpace(entry.CoffeeVarietal)
	entry.ProcessingMethod = strings.TrimSpace(entry.ProcessingMethod)
	entry.DaysSinceRoast = strings.TrimSpace(entry.DaysSinceRoast)
	entry.RoasterID = strings.TrimSpace(entry.RoasterID)
	entry.Roaster = strings.TrimSpace(entry.Roaster)
	entry.BrewMethod = strings.TrimSpace(entry.BrewMethod)
	entry.Ratio = strings.TrimSpace(entry.Ratio)
	entry.Grinder = strings.TrimSpace(entry.Grinder)
	entry.GrindSetting = strings.TrimSpace(entry.GrindSetting)
	entry.Dose = strings.TrimSpace(entry.Dose)
	entry.YieldAmount = strings.TrimSpace(entry.YieldAmount)
	entry.WaterTemperature = strings.TrimSpace(entry.WaterTemperature)
	entry.BrewTime = strings.TrimSpace(entry.BrewTime)
	entry.BloomTime = strings.TrimSpace(entry.BloomTime)
	entry.BloomWater = strings.TrimSpace(entry.BloomWater)
	entry.PourNotes = strings.TrimSpace(entry.PourNotes)
	entry.RoastLevel = strings.TrimSpace(entry.RoastLevel)
	entry.Notes = strings.TrimSpace(entry.Notes)
	entry.TastingNotes = strings.TrimSpace(entry.TastingNotes)

	if entry.Notes == "" {
		entry.Notes = entry.TastingNotes
	}
	entry.TastingNotes = entry.Notes

	if entry.RoasterID == "" && entry.Roaster != "" {
		entry.RoasterID = slugifyCoffeeValue(entry.Roaster)
	}
}

func normalizeCoffeeRoaster(roaster *coffeedata.Roaster) {
	roaster.ID = strings.TrimSpace(roaster.ID)
	roaster.Roaster = strings.TrimSpace(roaster.Roaster)

	if roaster.ID == "" && roaster.Roaster != "" {
		roaster.ID = slugifyCoffeeValue(roaster.Roaster)
	}
}

func applyCoffeeEntryPatch(entry *coffeedata.Entry, patch *coffeeEntryPatch) {
	if patch.Date != nil {
		entry.Date = *patch.Date
	}
	if patch.CoffeeName != nil {
		entry.CoffeeName = *patch.CoffeeName
	}
	if patch.Origin != nil {
		entry.Origin = *patch.Origin
	}
	if patch.CoffeeVarietal != nil {
		entry.CoffeeVarietal = *patch.CoffeeVarietal
	}
	if patch.ProcessingMethod != nil {
		entry.ProcessingMethod = *patch.ProcessingMethod
	}
	if patch.DaysSinceRoast != nil {
		entry.DaysSinceRoast = *patch.DaysSinceRoast
	}
	if patch.RoasterID != nil {
		entry.RoasterID = *patch.RoasterID
	}
	if patch.Roaster != nil {
		entry.Roaster = *patch.Roaster
		if patch.RoasterID == nil {
			entry.RoasterID = ""
		}
	}
	if patch.BrewMethod != nil {
		entry.BrewMethod = *patch.BrewMethod
	}
	if patch.Ratio != nil {
		entry.Ratio = *patch.Ratio
	}
	if patch.Grinder != nil {
		entry.Grinder = *patch.Grinder
	}
	if patch.GrindSetting != nil {
		entry.GrindSetting = *patch.GrindSetting
	}
	if patch.Dose != nil {
		entry.Dose = *patch.Dose
	}
	if patch.YieldAmount != nil {
		entry.YieldAmount = *patch.YieldAmount
	}
	if patch.WaterTemperature != nil {
		entry.WaterTemperature = *patch.WaterTemperature
	}
	if patch.BrewTime != nil {
		entry.BrewTime = *patch.BrewTime
	}
	if patch.BloomTime != nil {
		entry.BloomTime = *patch.BloomTime
	}
	if patch.BloomWater != nil {
		entry.BloomWater = *patch.BloomWater
	}
	if patch.PourNotes != nil {
		entry.PourNotes = *patch.PourNotes
	}
	if patch.RoastLevel != nil {
		entry.RoastLevel = *patch.RoastLevel
	}
	if patch.Notes != nil {
		entry.Notes = *patch.Notes
	}
	if patch.TastingNotes != nil && patch.Notes == nil {
		entry.Notes = *patch.TastingNotes
	}
	if patch.Rating != nil {
		entry.Rating = *patch.Rating
	}
}

func isValidCoffeeEntry(entry *coffeedata.Entry) bool {
	requiredFields := []string{
		entry.Date,
		entry.CoffeeName,
		entry.RoasterID,
		entry.Roaster,
		entry.BrewMethod,
		entry.Ratio,
		entry.Grinder,
		entry.GrindSetting,
		entry.Notes,
	}
	for _, field := range requiredFields {
		if field == "" {
			return false
		}
	}

	if _, err := time.Parse("2006-01-02", entry.Date); err != nil {
		return false
	}

	if entry.DaysSinceRoast != "" {
		daysSinceRoast, err := strconv.Atoi(entry.DaysSinceRoast)
		if err != nil || daysSinceRoast < 0 {
			return false
		}
	}

	return entry.Rating >= 0 && entry.Rating <= 5
}

func isValidCoffeeRoaster(roaster *coffeedata.Roaster) bool {
	return roaster.ID != "" && roaster.Roaster != ""
}

func slugifyCoffeeValue(value string) string {
	var builder strings.Builder
	lastWasDash := false

	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastWasDash = false
			continue
		}

		if builder.Len() > 0 && !lastWasDash {
			builder.WriteByte('-')
			lastWasDash = true
		}
	}

	return strings.Trim(builder.String(), "-")
}
