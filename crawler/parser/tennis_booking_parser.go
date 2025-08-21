package parser

import (
	"context"
	"fmt"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/crawler/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TennisBookingParser parses venue data from tennis_booking source
type TennisBookingParser struct {
	mongodb *mongo.Database
}

// NewTennisBookingParser creates a new parser for tennis_booking
func NewTennisBookingParser(mongodb *mongo.Database) *TennisBookingParser {
	return &TennisBookingParser{
		mongodb: mongodb,
	}
}

// GetSource returns the source name
func (p *TennisBookingParser) GetSource() string {
	return "tennis_booking"
}

// Parse parses raw data and returns venue snapshots
// For tennis_booking, we query the ground_board_periods collection
func (p *TennisBookingParser) Parse(taskID string, rawData map[string]interface{}) ([]types.VenueSnapshot, error) {
	snapshots := []types.VenueSnapshot{}
	now := time.Now()
	ctx := context.Background()
	
	// Query ground_board_periods collection for recent data
	// Look for data inserted in the last 5 minutes
	collection := p.mongodb.Collection("ground_board_periods")
	
	// Build query filter
	filter := bson.M{
		"project_id": "tennis_booking",
		"crawled_at": bson.M{
			"$gte": now.Add(-5 * time.Minute).Format("2006-01-02 15:04:05"),
		},
	}
	
	// Query options
	findOptions := options.Find().SetSort(bson.D{{"crawled_at", -1}})
	
	// Execute query
	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return snapshots, fmt.Errorf("failed to query ground_board_periods: %w", err)
	}
	defer cursor.Close(ctx)
	
	// Process each period record
	for cursor.Next(ctx) {
		var period bson.M
		if err := cursor.Decode(&period); err != nil {
			continue
		}
		
		// Extract time slot from start_at and end_at
		timeSlot := extractTimeSlot(period["start_at"], period["end_at"])
		
		// Only process evening slots (19:00-22:00)
		if !isEveningSlot(timeSlot) {
			continue
		}
		
		// Convert to VenueSnapshot
		snapshot := types.VenueSnapshot{
			TaskID:    taskID,
			Source:    p.GetSource(),
			TimeSlot:  timeSlot,
			VenueID:   getStringValue(period["ground_id"]),
			VenueName: getStringValue(period["ground_name"]),
			Status:    getStringValue(period["status_name"]), // already normalized in Lua
			Price:     getFloatValue(period["price"]),
			Available: calculateAvailable(getStringValue(period["status_name"])),
			Capacity:  1, // Single court
			RawData:   period,
			CrawledAt: now,
		}
		
		snapshots = append(snapshots, snapshot)
	}
	
	if err := cursor.Err(); err != nil {
		return snapshots, fmt.Errorf("cursor error: %w", err)
	}
	
	return snapshots, nil
}