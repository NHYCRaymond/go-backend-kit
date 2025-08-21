package utils

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// WeekData represents a single day in the week array
type WeekData struct {
	Week string `json:"week"` // "今天", "明天", "后天", or "周X"
	Time string `json:"time"` // Date in YYYY-MM-DD format
	Date string `json:"date"` // Date in M.D format
}

// VenueRequestParams holds parameters for venue requests
type VenueRequestParams struct {
	VID    string     `json:"vid"`
	PID    string     `json:"pid"`
	Date   string     `json:"date"`
	Week   []WeekData `json:"week"`
	OpenID string     `json:"openid"`
}

// GenerateWeekArray generates an array of week data for the next 7 days
func GenerateWeekArray() []WeekData {
	weekNames := []string{"今天", "明天", "后天"}
	weekDays := []string{"日", "一", "二", "三", "四", "五", "六"}
	week := make([]WeekData, 0, 7)
	
	today := time.Now()
	
	for i := 0; i < 7; i++ {
		date := today.AddDate(0, 0, i)
		
		var weekLabel string
		if i < 3 {
			weekLabel = weekNames[i]
		} else {
			weekLabel = fmt.Sprintf("周%s", weekDays[int(date.Weekday())])
		}
		
		week = append(week, WeekData{
			Week: weekLabel,
			Time: date.Format("2006-01-02"),
			Date: fmt.Sprintf("%d.%d", date.Month(), date.Day()),
		})
	}
	
	return week
}

// BuildVenueRequestBody builds a URL-encoded request body for venue API
func BuildVenueRequestBody(vid, pid, openid string) (string, error) {
	// Use defaults if not provided
	if vid == "" {
		vid = "1557"
	}
	if pid == "" {
		pid = "18"
	}
	if openid == "" {
		openid = "o_PUd7UvwOKaxG8AHYDo6X962bvg"
	}
	
	// Generate current date and week array
	currentDate := time.Now().Format("2006-01-02")
	weekArray := GenerateWeekArray()
	
	// Convert week array to JSON
	weekJSON, err := json.Marshal(weekArray)
	if err != nil {
		return "", fmt.Errorf("failed to marshal week array: %w", err)
	}
	
	// Build URL-encoded parameters
	params := url.Values{}
	params.Set("vid", vid)
	params.Set("pid", pid)
	params.Set("date", currentDate)
	params.Set("week", string(weekJSON))
	params.Set("openid", openid)
	
	return params.Encode(), nil
}

// UpdateVenueTaskBody updates the request body of a venue task with current dates
// This can be called before task execution to ensure dates are current
func UpdateVenueTaskBody(task map[string]interface{}) (string, error) {
	// Extract parameters from task config
	config, ok := task["config"].(map[string]interface{})
	if !ok {
		config = make(map[string]interface{})
	}
	
	params, ok := config["parameters"].(map[string]interface{})
	if !ok {
		params = make(map[string]interface{})
	}
	
	// Get venue parameters
	vid, _ := params["vid"].(string)
	pid, _ := params["pid"].(string)
	openid, _ := params["openid"].(string)
	
	// Build new request body
	return BuildVenueRequestBody(vid, pid, openid)
}

// ParseVenueRequestBody parses a URL-encoded venue request body
func ParseVenueRequestBody(body string) (*VenueRequestParams, error) {
	values, err := url.ParseQuery(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}
	
	params := &VenueRequestParams{
		VID:    values.Get("vid"),
		PID:    values.Get("pid"),
		Date:   values.Get("date"),
		OpenID: values.Get("openid"),
	}
	
	// Parse week JSON
	weekJSON := values.Get("week")
	if weekJSON != "" {
		if err := json.Unmarshal([]byte(weekJSON), &params.Week); err != nil {
			return nil, fmt.Errorf("failed to unmarshal week data: %w", err)
		}
	}
	
	return params, nil
}