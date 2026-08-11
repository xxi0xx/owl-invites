package invitation

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/xxi0xx/owl-invites/internal/errcode"
)

const (
	MaxImportBytes      = 512 << 10 // 512 KiB; keeps normalized commit JSON below the global API limit.
	MaxImportRows       = 5000
	MaxImportHouseholds = 1000
	MaxHouseholdGuests  = 100
)

var importColumns = []string{
	"household_key",
	"household_label",
	"contact_email",
	"contact_phone",
	"preferred_delivery",
	"additional_guest_allowance",
	"guest_name",
}

var requiredImportColumns = map[string]bool{
	"household_key":              true,
	"household_label":            true,
	"preferred_delivery":         true,
	"additional_guest_allowance": true,
	"guest_name":                 true,
}

const importTemplateCSV = "household_key,household_label,contact_email,contact_phone,preferred_delivery,additional_guest_allowance,guest_name\r\n" +
	"smith,Smith Family,smith@example.com,,email,1,Jane Smith\r\n" +
	"smith,Smith Family,smith@example.com,,email,1,John Smith\r\n" +
	"garcia,Garcia Family,maria@example.com,,email,0,Maria Garcia\r\n"

// PreviewImportCSV parses and normalizes household rows without writing to the
// database. Validation failures are returned in the preview so the organizer
// can correct multiple rows in one pass.
func PreviewImportCSV(reader io.Reader) (*ImportPreview, error) {
	preview := emptyImportPreview()
	data, err := io.ReadAll(io.LimitReader(reader, MaxImportBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read invitation import: %w", err)
	}
	if len(data) > MaxImportBytes {
		preview.Errors = append(preview.Errors, ImportIssue{Message: fmt.Sprintf("CSV file exceeds the %d byte limit", MaxImportBytes)})
		return preview, nil
	}
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		data = data[3:]
	}
	if !utf8.Valid(data) {
		preview.Errors = append(preview.Errors, ImportIssue{Message: "CSV must be valid UTF-8"})
		return preview, nil
	}

	parser := csv.NewReader(bytes.NewReader(data))
	parser.FieldsPerRecord = -1
	parser.ReuseRecord = false
	header, err := parser.Read()
	if err == io.EOF {
		preview.Errors = append(preview.Errors, ImportIssue{Message: "CSV is empty"})
		return preview, nil
	}
	if err != nil {
		preview.Errors = append(preview.Errors, ImportIssue{Message: "malformed CSV header: " + err.Error()})
		return preview, nil
	}
	indexes := validateImportHeader(header, preview)
	if len(preview.Errors) > 0 {
		return preview, nil
	}

	type householdAccumulator struct {
		household ImportHousehold
		firstRow  int
	}
	byKey := make(map[string]*householdAccumulator)
	order := make([]string, 0)
	emailKeys := make(map[string]map[string]bool)
	rowNumber := 1
	for {
		record, readErr := parser.Read()
		if readErr == io.EOF {
			break
		}
		rowNumber++
		if readErr != nil {
			preview.Errors = append(preview.Errors, ImportIssue{Row: rowNumber, Message: "malformed CSV: " + readErr.Error()})
			break
		}
		if rowNumber-1 > MaxImportRows {
			preview.Errors = append(preview.Errors, ImportIssue{Message: fmt.Sprintf("CSV exceeds the %d data row limit", MaxImportRows)})
			break
		}
		if len(record) != len(header) {
			preview.Errors = append(preview.Errors, ImportIssue{Row: rowNumber, Message: fmt.Sprintf("expected %d columns, found %d", len(header), len(record))})
			continue
		}

		value := func(column string) string {
			index, ok := indexes[column]
			if !ok {
				return ""
			}
			return strings.TrimSpace(record[index])
		}
		key := value("household_key")
		label := value("household_label")
		emailRaw := value("contact_email")
		phoneRaw := value("contact_phone")
		method := strings.ToLower(value("preferred_delivery"))
		allowanceRaw := value("additional_guest_allowance")
		guestName := value("guest_name")

		rowErrors := validateImportRow(rowNumber, key, label, emailRaw, phoneRaw, method, allowanceRaw, guestName)
		preview.Errors = append(preview.Errors, rowErrors...)
		if len(rowErrors) > 0 {
			continue
		}
		allowance, _ := strconv.Atoi(allowanceRaw)
		var email, phone *string
		if emailRaw != "" {
			normalized := normalizeEmail(emailRaw)
			email = &normalized
		}
		if phoneRaw != "" {
			normalized := normalizePhone(phoneRaw)
			phone = &normalized
		}
		normalized := ImportHousehold{
			HouseholdKey: key, HouseholdLabel: label, ContactEmail: email,
			ContactPhone: phone, PreferredDelivery: method,
			AdditionalGuestAllowance: allowance, AssignedGuestNames: []string{guestName},
		}
		accumulator := byKey[key]
		if accumulator == nil {
			if len(byKey) >= MaxImportHouseholds {
				preview.Errors = append(preview.Errors, ImportIssue{Row: rowNumber, Field: "household_key", Message: fmt.Sprintf("CSV exceeds the %d household limit", MaxImportHouseholds)})
				continue
			}
			accumulator = &householdAccumulator{household: normalized, firstRow: rowNumber}
			byKey[key] = accumulator
			order = append(order, key)
		} else if conflicts := householdConflicts(accumulator.household, normalized); len(conflicts) > 0 {
			for _, field := range conflicts {
				preview.Errors = append(preview.Errors, ImportIssue{Row: rowNumber, Field: field,
					Message: fmt.Sprintf("conflicts with household %q first defined on row %d", key, accumulator.firstRow)})
			}
			continue
		} else if len(accumulator.household.AssignedGuestNames) >= MaxHouseholdGuests {
			preview.Errors = append(preview.Errors, ImportIssue{Row: rowNumber, Field: "guest_name", Message: fmt.Sprintf("household %q exceeds the %d assigned guest limit", key, MaxHouseholdGuests)})
			continue
		} else {
			accumulator.household.AssignedGuestNames = append(accumulator.household.AssignedGuestNames, guestName)
		}
		if email != nil {
			keys := emailKeys[*email]
			if keys == nil {
				keys = make(map[string]bool)
				emailKeys[*email] = keys
			}
			keys[key] = true
		}
	}

	for _, key := range order {
		preview.Households = append(preview.Households, byKey[key].household)
		preview.AssignedGuestCount += len(byKey[key].household.AssignedGuestNames)
	}
	preview.HouseholdCount = len(preview.Households)
	for email, keys := range emailKeys {
		if len(keys) > 1 {
			preview.Warnings = append(preview.Warnings, ImportIssue{Field: "contact_email",
				Message: fmt.Sprintf("%s is used by %d households; they will remain separate invitations", email, len(keys))})
		}
	}
	return preview, nil
}

func emptyImportPreview() *ImportPreview {
	return &ImportPreview{Households: []ImportHousehold{}, Errors: []ImportIssue{}, Warnings: []ImportIssue{}}
}

func validateImportHeader(header []string, preview *ImportPreview) map[string]int {
	indexes := make(map[string]int, len(header))
	supported := make(map[string]bool, len(importColumns))
	for _, column := range importColumns {
		supported[column] = true
	}
	for index, raw := range header {
		column := strings.TrimSpace(strings.ToLower(raw))
		if column == "" {
			preview.Errors = append(preview.Errors, ImportIssue{Message: "CSV contains a blank header"})
			continue
		}
		if !supported[column] {
			preview.Errors = append(preview.Errors, ImportIssue{Field: column, Message: "unsupported CSV column"})
			continue
		}
		if _, exists := indexes[column]; exists {
			preview.Errors = append(preview.Errors, ImportIssue{Field: column, Message: "duplicate CSV header"})
			continue
		}
		indexes[column] = index
	}
	for column := range requiredImportColumns {
		if _, exists := indexes[column]; !exists {
			preview.Errors = append(preview.Errors, ImportIssue{Field: column, Message: "required CSV column is missing"})
		}
	}
	return indexes
}

func validateImportRow(row int, key, label, email, phone, method, allowanceRaw, guestName string) []ImportIssue {
	issues := make([]ImportIssue, 0)
	add := func(field, message string) {
		issues = append(issues, ImportIssue{Row: row, Field: field, Message: message})
	}
	if key == "" {
		add("household_key", "household key is required")
	} else if len(key) > 100 {
		add("household_key", "household key must be 100 characters or fewer")
	}
	if label == "" {
		add("household_label", "household label is required")
	} else if len(label) > 200 {
		add("household_label", "household label must be 200 characters or fewer")
	}
	if guestName == "" {
		add("guest_name", "guest name is required")
	} else if len(guestName) > 200 {
		add("guest_name", "guest name must be 200 characters or fewer")
	}
	allowance, err := strconv.Atoi(allowanceRaw)
	if err != nil || allowance < 0 || allowance > 50 {
		add("additional_guest_allowance", "additional guest allowance must be an integer between 0 and 50")
	}
	var emailPtr, phonePtr *string
	if email != "" {
		emailPtr = &email
	}
	if phone != "" {
		phonePtr = &phone
	}
	if _, _, _, err := validateContact(emailPtr, phonePtr, method); err != nil {
		add("preferred_delivery", err.Error())
	}
	return issues
}

func householdConflicts(first, next ImportHousehold) []string {
	conflicts := make([]string, 0)
	if first.HouseholdLabel != next.HouseholdLabel {
		conflicts = append(conflicts, "household_label")
	}
	if stringValue(first.ContactEmail) != stringValue(next.ContactEmail) {
		conflicts = append(conflicts, "contact_email")
	}
	if stringValue(first.ContactPhone) != stringValue(next.ContactPhone) {
		conflicts = append(conflicts, "contact_phone")
	}
	if first.PreferredDelivery != next.PreferredDelivery {
		conflicts = append(conflicts, "preferred_delivery")
	}
	if first.AdditionalGuestAllowance != next.AdditionalGuestAllowance {
		conflicts = append(conflicts, "additional_guest_allowance")
	}
	return conflicts
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// CommitImport revalidates normalized preview data and atomically creates new
// private invitations. It never searches existing invitations and never sends
// email.
func (s *Service) CommitImport(ctx context.Context, eventID, creatorUserID string, request ImportCommitRequest) (*ImportCommitResult, error) {
	validated, err := validateImportCommit(request)
	if err != nil {
		return nil, err
	}
	records := make([]ImportRecord, 0, len(validated))
	result := &ImportCommitResult{HouseholdCount: len(validated), InvitationIDs: make([]string, 0, len(validated))}
	for _, input := range validated {
		accessID, err := randomToken(18)
		if err != nil {
			return nil, err
		}
		creator := creatorUserID
		invitation := &Invitation{
			ID: uuid.Must(uuid.NewV7()).String(), EventID: eventID, Label: input.HouseholdLabel,
			ContactEmail: input.ContactEmail, ContactPhone: input.ContactPhone,
			PreferredDeliveryMethod:  input.PreferredDelivery,
			AdditionalGuestAllowance: input.AdditionalGuestAllowance, Source: SourcePrivate,
			AccessID: accessID, TokenVersion: 1, CreatedByUserID: &creator,
		}
		guests := make([]*Guest, 0, len(input.AssignedGuestNames))
		for index, name := range input.AssignedGuestNames {
			guests = append(guests, &Guest{ID: uuid.Must(uuid.NewV7()).String(), Name: name,
				Origin: GuestOriginAssigned, SortOrder: index})
		}
		records = append(records, ImportRecord{Invitation: invitation, Guests: guests})
		result.InvitationIDs = append(result.InvitationIDs, invitation.ID)
		result.AssignedGuestCount += len(guests)
	}
	if err := s.store.Import(ctx, records); err != nil {
		return nil, err
	}
	return result, nil
}

func validateImportCommit(request ImportCommitRequest) ([]ImportHousehold, error) {
	if len(request.Households) == 0 {
		return nil, errcode.Validationf("at least one household is required")
	}
	if len(request.Households) > MaxImportHouseholds {
		return nil, errcode.Validationf("import exceeds the %d household limit", MaxImportHouseholds)
	}
	seen := make(map[string]bool, len(request.Households))
	totalGuests := 0
	validated := make([]ImportHousehold, 0, len(request.Households))
	for index, raw := range request.Households {
		input := raw
		input.HouseholdKey = strings.TrimSpace(input.HouseholdKey)
		input.HouseholdLabel = strings.TrimSpace(input.HouseholdLabel)
		input.PreferredDelivery = strings.ToLower(strings.TrimSpace(input.PreferredDelivery))
		if seen[input.HouseholdKey] {
			return nil, errcode.Validationf("duplicate household key at household %d", index+1)
		}
		seen[input.HouseholdKey] = true
		if input.HouseholdKey == "" || len(input.HouseholdKey) > 100 {
			return nil, errcode.Validationf("invalid household key at household %d", index+1)
		}
		if input.HouseholdLabel == "" || len(input.HouseholdLabel) > 200 {
			return nil, errcode.Validationf("invalid household label at household %d", index+1)
		}
		if input.AdditionalGuestAllowance < 0 || input.AdditionalGuestAllowance > 50 {
			return nil, errcode.Validationf("invalid additional guest allowance at household %d", index+1)
		}
		if len(input.AssignedGuestNames) == 0 || len(input.AssignedGuestNames) > MaxHouseholdGuests {
			return nil, errcode.Validationf("household %d must contain between 1 and %d assigned guests", index+1, MaxHouseholdGuests)
		}
		email, phone, method, err := validateContact(input.ContactEmail, input.ContactPhone, input.PreferredDelivery)
		if err != nil {
			return nil, errcode.Validationf("household %d: %v", index+1, err)
		}
		input.ContactEmail, input.ContactPhone, input.PreferredDelivery = email, phone, method
		for guestIndex, rawName := range input.AssignedGuestNames {
			name := strings.TrimSpace(rawName)
			if name == "" || len(name) > 200 {
				return nil, errcode.Validationf("invalid assigned guest name at household %d guest %d", index+1, guestIndex+1)
			}
			input.AssignedGuestNames[guestIndex] = name
		}
		totalGuests += len(input.AssignedGuestNames)
		if totalGuests > MaxImportRows {
			return nil, errcode.Validationf("import exceeds the %d assigned guest limit", MaxImportRows)
		}
		validated = append(validated, input)
	}
	return validated, nil
}
