package parser

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/types"
)


// VenueMassageSiteParser parses venue data from venue_massage_site source
type VenueMassageSiteParser struct{}

// NewVenueMassageSiteParser creates a new parser for venue_massage_site
func NewVenueMassageSiteParser() *VenueMassageSiteParser {
	return &VenueMassageSiteParser{}
}

// GetSource returns the source name
func (p *VenueMassageSiteParser) GetSource() string {
	return "venue_massage_site"
}

// Parse parses raw data and returns venue snapshots
func (p *VenueMassageSiteParser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	now := time.Now()
	
	// Parse venue data - structure depends on actual API response
	if venues, ok := rawData["venues"].([]interface{}); ok {
		for _, venue := range venues {
			if venueMap, ok := venue.(map[string]interface{}); ok {
				timeSlot := getStringValue(venueMap["time_slot"])
				if !isEveningSlot(timeSlot) {
					continue
				}
				
				snapshot := types.VenueSnapshot{
					TaskID:    taskID,
					Source:    p.GetSource(),
					TimeSlot:  timeSlot,
					VenueID:   getStringValue(venueMap["venue_id"]),
					VenueName: getStringValue(venueMap["venue_name"]),
					Status:    getStringValue(venueMap["status"]),
					Price:     getFloatValue(venueMap["price"]),
					Available: getIntValue(venueMap["available"]),
					Capacity:  getIntValue(venueMap["capacity"]),
					RawData:   venueMap,
					CrawledAt: now,
				}
				
				snapshots = append(snapshots, snapshot)
			}
		}
	}
	
	return snapshots, nil
}

// PospalVenue5662377Parser parses venue data from pospal_venue_5662377 source
type PospalVenue5662377Parser struct{}

// NewPospalVenue5662377Parser creates a new parser for pospal_venue_5662377
func NewPospalVenue5662377Parser() *PospalVenue5662377Parser {
	return &PospalVenue5662377Parser{}
}

// GetSource returns the source name
func (p *PospalVenue5662377Parser) GetSource() string {
	return "pospal_venue_5662377"
}

// Parse parses raw data and returns venue snapshots
func (p *PospalVenue5662377Parser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	now := time.Now()
	
	// Parse Pospal venue data
	if venues, ok := rawData["venues"].([]interface{}); ok {
		for _, venue := range venues {
			if venueMap, ok := venue.(map[string]interface{}); ok {
				timeSlot := getStringValue(venueMap["time_slot"])
				if !isEveningSlot(timeSlot) {
					continue
				}
				
				// Convert Pospal specific status codes
				status := parseStatus(venueMap["status"])
				
				snapshot := types.VenueSnapshot{
					TaskID:    taskID,
					Source:    p.GetSource(),
					TimeSlot:  timeSlot,
					VenueID:   getStringValue(venueMap["venue_id"]),
					VenueName: getStringValue(venueMap["venue_name"]),
					Status:    status,
					Price:     getFloatValue(venueMap["price"]),
					Available: getIntValue(venueMap["available"]),
					Capacity:  getIntValue(venueMap["capacity"]),
					RawData:   venueMap,
					CrawledAt: now,
				}
				
				snapshots = append(snapshots, snapshot)
			}
		}
	}
	
	return snapshots, nil
}

// PospalVenueClassroomParser parses venue data from pospal_venue_classroom source
type PospalVenueClassroomParser struct{}

// NewPospalVenueClassroomParser creates a new parser for pospal_venue_classroom
func NewPospalVenueClassroomParser() *PospalVenueClassroomParser {
	return &PospalVenueClassroomParser{}
}

// GetSource returns the source name
func (p *PospalVenueClassroomParser) GetSource() string {
	return "pospal_venue_classroom"
}

// Parse parses raw data and returns venue snapshots
func (p *PospalVenueClassroomParser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	now := time.Now()
	
	// Parse classroom venue data
	if classrooms, ok := rawData["classrooms"].([]interface{}); ok {
		for _, classroom := range classrooms {
			if classroomMap, ok := classroom.(map[string]interface{}); ok {
				timeSlot := getStringValue(classroomMap["time_slot"])
				if !isEveningSlot(timeSlot) {
					continue
				}
				
				snapshot := types.VenueSnapshot{
					TaskID:    taskID,
					Source:    p.GetSource(),
					TimeSlot:  timeSlot,
					VenueID:   getStringValue(classroomMap["classroom_id"]),
					VenueName: getStringValue(classroomMap["classroom_name"]),
					Status:    parseStatus(classroomMap["status"]),
					Price:     getFloatValue(classroomMap["price"]),
					Available: getIntValue(classroomMap["available"]),
					Capacity:  getIntValue(classroomMap["capacity"]),
					RawData:   classroomMap,
					CrawledAt: now,
				}
				
				snapshots = append(snapshots, snapshot)
			}
		}
	}
	
	// Also check for generic venues field
	if venues, ok := rawData["venues"].([]interface{}); ok {
		for _, venue := range venues {
			if venueMap, ok := venue.(map[string]interface{}); ok {
				timeSlot := getStringValue(venueMap["time_slot"])
				if !isEveningSlot(timeSlot) {
					continue
				}
				
				snapshot := types.VenueSnapshot{
					TaskID:    taskID,
					Source:    p.GetSource(),
					TimeSlot:  timeSlot,
					VenueID:   getStringValue(venueMap["venue_id"]),
					VenueName: getStringValue(venueMap["venue_name"]),
					Status:    parseStatus(venueMap["status"]),
					Price:     getFloatValue(venueMap["price"]),
					Available: getIntValue(venueMap["available"]),
					Capacity:  getIntValue(venueMap["capacity"]),
					RawData:   venueMap,
					CrawledAt: now,
				}
				
				snapshots = append(snapshots, snapshot)
			}
		}
	}
	
	return snapshots, nil
}


// Helper functions

// extractTimeSlot extracts time slot from start and end times
func extractTimeSlot(startAt, endAt interface{}) string {
	startStr := getStringValue(startAt)
	endStr := getStringValue(endAt)
	
	// Extract time part from datetime strings
	// Format: "2025-08-19 19:00:00" -> "19:00"
	if len(startStr) >= 16 && len(endStr) >= 16 {
		startTime := startStr[11:16] // Extract "19:00"
		endTime := endStr[11:16]     // Extract "20:00"
		return fmt.Sprintf("%s-%s", startTime, endTime)
	}
	
	return ""
}

// isEveningSlot checks if the time slot is between 19:00 and 22:00
func isEveningSlot(timeSlot string) bool {
	// Check various time slot formats
	eveningHours := []string{"19:", "20:", "21:"}
	
	for _, hour := range eveningHours {
		if strings.Contains(timeSlot, hour) {
			return true
		}
	}
	
	// Also check if it ends at 22:00
	if strings.Contains(timeSlot, "22:00") || strings.Contains(timeSlot, "22:30") {
		return true
	}
	
	return false
}

// parseStatus converts various status formats to standard format
func parseStatus(status interface{}) string {
	switch v := status.(type) {
	case string:
		return v
	case float64:
		if v == 1 {
			return "available"
		} else if v == 2 {
			return "booked"
		}
		return "unavailable"
	case int:
		if v == 1 {
			return "available"
		} else if v == 2 {
			return "booked"
		}
		return "unavailable"
	default:
		return "unavailable"
	}
}

// calculateAvailable calculates available count based on status
func calculateAvailable(status string) int {
	if status == "available" {
		return 1
	}
	return 0
}

// getStringValue safely gets string value from interface
func getStringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// getFloatValue safely gets float value from interface
func getFloatValue(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

// getIntValue safely gets int value from interface
func getIntValue(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case string:
		i, _ := strconv.Atoi(val)
		return i
	default:
		return 0
	}
}