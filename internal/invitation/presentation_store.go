package invitation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var guestColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)
var guestFontPattern = regexp.MustCompile(`^[A-Za-z0-9 ,\-_'"]+$`)

var guestTemplates = map[string]bool{
	"balloon-party": true, "confetti": true, "unicorn-magic": true,
	"superhero": true, "garden-picnic": true, "elegant-affair": true,
	"clean-minimal": true, "tropical-vibes": true, "vintage-retro": true,
	"chalkboard": true,
}

func (s *Store) loadGuestPresentation(ctx context.Context, eventID string) (*GuestPresentation, error) {
	var templateID, heading, body, footer, primary, secondary, font, customData string
	err := s.db.QueryRowContext(ctx, `SELECT template_id, heading, body, footer,
		primary_color, secondary_color, font, custom_data
		FROM invite_cards WHERE event_id = ?`, eventID).Scan(&templateID, &heading,
		&body, &footer, &primary, &secondary, &font, &customData)
	if err == sql.ErrNoRows {
		return defaultGuestPresentation(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("load guest invitation presentation: %w", err)
	}
	if !guestTemplates[templateID] {
		templateID = "clean-minimal"
	}
	if !guestColorPattern.MatchString(primary) {
		primary = "#E54666"
	}
	if !guestColorPattern.MatchString(secondary) {
		secondary = "#f472b6"
	}
	if len(font) > 100 || !guestFontPattern.MatchString(font) {
		font = "Inter"
	}
	return &GuestPresentation{
		TemplateID: templateID, Heading: heading, Body: body, Footer: footer,
		PrimaryColor: primary, SecondaryColor: secondary, Font: font,
		BackgroundImage: guestBackgroundImage(customData),
	}, nil
}

func defaultGuestPresentation() *GuestPresentation {
	return &GuestPresentation{
		TemplateID: "clean-minimal", PrimaryColor: "#E54666",
		SecondaryColor: "#f472b6", Font: "Inter",
	}
}

func guestBackgroundImage(customData string) string {
	var data struct {
		BackgroundImage string `json:"backgroundImage"`
	}
	if json.Unmarshal([]byte(customData), &data) != nil {
		return ""
	}
	value := strings.TrimSpace(data.BackgroundImage)
	if value == "" || strings.ContainsAny(value, "()\"'<>\\") {
		return ""
	}
	if strings.HasPrefix(value, "/api/v1/uploads/") && !strings.Contains(value, "..") {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return value
}
