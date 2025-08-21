package parser

import (
	"fmt"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/types"
)

// BaseVenueParser provides common functionality for venue parsers
type BaseVenueParser struct {
	source string
}

// NewBaseVenueParser creates a new base venue parser
func NewBaseVenueParser(source string) *BaseVenueParser {
	return &BaseVenueParser{
		source: source,
	}
}

// GetSource returns the parser's source
func (p *BaseVenueParser) GetSource() string {
	return p.source
}

// TennisVenueParser parses tennis venue data
type TennisVenueParser struct {
	*BaseVenueParser
}

// NewTennisVenueParser creates a tennis venue parser
func NewTennisVenueParser() *TennisVenueParser {
	return &TennisVenueParser{
		BaseVenueParser: NewBaseVenueParser("tennis_court"),
	}
}

// Parse parses tennis venue data from crawler result
func (p *TennisVenueParser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	
	// Extract venues from raw data
	venues, ok := rawData["venues"].([]interface{})
	if !ok {
		return snapshots, fmt.Errorf("no venues found in data")
	}
	
	now := time.Now()
	
	for _, v := range venues {
		venue, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		
		// Extract venue fields
		venueID, _ := venue["venue_id"].(string)
		venueName, _ := venue["venue_name"].(string)
		timeSlot, _ := venue["time_slot"].(string)
		status, _ := venue["status"].(string)
		
		// Extract numeric fields with type conversion
		var price float64
		if p, ok := venue["price"].(float64); ok {
			price = p
		} else if p, ok := venue["price"].(int); ok {
			price = float64(p)
		}
		
		var available, capacity int
		if a, ok := venue["available"].(float64); ok {
			available = int(a)
		} else if a, ok := venue["available"].(int); ok {
			available = a
		}
		
		if c, ok := venue["capacity"].(float64); ok {
			capacity = int(c)
		} else if c, ok := venue["capacity"].(int); ok {
			capacity = c
		}
		
		snapshot := types.VenueSnapshot{
			TaskID:     taskID,
			Source:     p.source,
			TimeSlot:   timeSlot,
			VenueID:    venueID,
			VenueName:  venueName,
			Status:     status,
			Price:      price,
			Capacity:   capacity,
			Available:  available,
			RawData:    venue,
			CrawledAt:  now,
		}
		
		snapshots = append(snapshots, snapshot)
	}
	
	return snapshots, nil
}

// BadmintonVenueParser parses badminton venue data
type BadmintonVenueParser struct {
	*BaseVenueParser
}

// NewBadmintonVenueParser creates a badminton venue parser
func NewBadmintonVenueParser() *BadmintonVenueParser {
	return &BadmintonVenueParser{
		BaseVenueParser: NewBaseVenueParser("badminton_court"),
	}
}

// Parse parses badminton venue data
func (p *BadmintonVenueParser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	
	// Extract courts data
	courts, ok := rawData["courts"].([]interface{})
	if !ok {
		return snapshots, fmt.Errorf("no courts found in data")
	}
	
	now := time.Now()
	
	for _, c := range courts {
		court, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		
		// Extract time slots
		slots, ok := court["time_slots"].([]interface{})
		if !ok {
			continue
		}
		
		courtID, _ := court["court_id"].(string)
		courtName, _ := court["court_name"].(string)
		
		for _, s := range slots {
			slot, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			
			timeSlot, _ := slot["time"].(string)
			status, _ := slot["status"].(string)
			
			// Price conversion
			var price float64
			if p, ok := slot["price"].(float64); ok {
				price = p
			} else if p, ok := slot["price"].(int); ok {
				price = float64(p)
			}
			
			// Determine availability based on status
			available := 0
			if status == "available" {
				available = 1
			}
			
			snapshot := types.VenueSnapshot{
				TaskID:     taskID,
				Source:     p.source,
				TimeSlot:   timeSlot,
				VenueID:    courtID,
				VenueName:  courtName,
				Status:     status,
				Price:      price,
				Capacity:   1,
				Available:  available,
				RawData:    slot,
				CrawledAt:  now,
			}
			
			snapshots = append(snapshots, snapshot)
		}
	}
	
	return snapshots, nil
}

// SwimmingPoolParser parses swimming pool venue data
type SwimmingPoolParser struct {
	*BaseVenueParser
}

// NewSwimmingPoolParser creates a swimming pool parser
func NewSwimmingPoolParser() *SwimmingPoolParser {
	return &SwimmingPoolParser{
		BaseVenueParser: NewBaseVenueParser("swimming_pool"),
	}
}

// Parse parses swimming pool data
func (p *SwimmingPoolParser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	
	// Extract sessions data
	sessions, ok := rawData["sessions"].([]interface{})
	if !ok {
		return snapshots, fmt.Errorf("no sessions found in data")
	}
	
	now := time.Now()
	poolName, _ := rawData["pool_name"].(string)
	
	for _, s := range sessions {
		session, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		
		sessionID, _ := session["session_id"].(string)
		timeSlot, _ := session["time_slot"].(string)
		
		// Extract numeric fields
		var price float64
		if p, ok := session["price"].(float64); ok {
			price = p
		} else if p, ok := session["price"].(int); ok {
			price = float64(p)
		}
		
		var available, capacity int
		if a, ok := session["available_spots"].(float64); ok {
			available = int(a)
		} else if a, ok := session["available_spots"].(int); ok {
			available = a
		}
		
		if c, ok := session["max_capacity"].(float64); ok {
			capacity = int(c)
		} else if c, ok := session["max_capacity"].(int); ok {
			capacity = c
		}
		
		// Determine status based on availability
		status := "available"
		if available == 0 {
			status = "full"
		}
		
		snapshot := types.VenueSnapshot{
			TaskID:     taskID,
			Source:     p.source,
			TimeSlot:   timeSlot,
			VenueID:    sessionID,
			VenueName:  poolName,
			Status:     status,
			Price:      price,
			Capacity:   capacity,
			Available:  available,
			RawData:    session,
			CrawledAt:  now,
		}
		
		snapshots = append(snapshots, snapshot)
	}
	
	return snapshots, nil
}

// GymVenueParser parses gym venue data
type GymVenueParser struct {
	*BaseVenueParser
}

// NewGymVenueParser creates a gym venue parser
func NewGymVenueParser() *GymVenueParser {
	return &GymVenueParser{
		BaseVenueParser: NewBaseVenueParser("gym"),
	}
}

// Parse parses gym venue data
func (p *GymVenueParser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	
	// Extract classes or sessions
	classes, ok := rawData["classes"].([]interface{})
	if !ok {
		return snapshots, fmt.Errorf("no classes found in data")
	}
	
	now := time.Now()
	gymName, _ := rawData["gym_name"].(string)
	
	for _, c := range classes {
		class, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		
		classID, _ := class["class_id"].(string)
		className, _ := class["class_name"].(string)
		timeSlot, _ := class["time"].(string)
		instructor, _ := class["instructor"].(string)
		
		// Extract numeric fields
		var price float64
		if p, ok := class["price"].(float64); ok {
			price = p
		} else if p, ok := class["price"].(int); ok {
			price = float64(p)
		}
		
		var available, capacity int
		if a, ok := class["available"].(float64); ok {
			available = int(a)
		} else if a, ok := class["available"].(int); ok {
			available = a
		}
		
		if c, ok := class["capacity"].(float64); ok {
			capacity = int(c)
		} else if c, ok := class["capacity"].(int); ok {
			capacity = c
		}
		
		// Determine status
		status := "available"
		if available == 0 {
			status = "full"
		} else if available < 5 {
			status = "limited"
		}
		
		// Build venue name with class and instructor
		venueName := fmt.Sprintf("%s - %s", gymName, className)
		if instructor != "" {
			venueName = fmt.Sprintf("%s (%s)", venueName, instructor)
		}
		
		snapshot := types.VenueSnapshot{
			TaskID:     taskID,
			Source:     p.source,
			TimeSlot:   timeSlot,
			VenueID:    classID,
			VenueName:  venueName,
			Status:     status,
			Price:      price,
			Capacity:   capacity,
			Available:  available,
			RawData:    class,
			CrawledAt:  now,
		}
		
		snapshots = append(snapshots, snapshot)
	}
	
	return snapshots, nil
}