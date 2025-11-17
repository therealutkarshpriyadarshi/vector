package search

import (
	"fmt"
	"math"
	"time"
)

// Filter represents a metadata filter that can be applied to search results
type Filter interface {
	// Match returns true if the given metadata passes the filter
	Match(metadata map[string]interface{}) bool
}

// FilterOperator defines the type of filter operation
type FilterOperator string

const (
	OpEquals      FilterOperator = "eq"       // Equals
	OpNotEquals   FilterOperator = "ne"       // Not equals
	OpGreaterThan FilterOperator = "gt"       // Greater than
	OpLessThan    FilterOperator = "lt"       // Less than
	OpGreaterOrEq FilterOperator = "gte"      // Greater than or equal
	OpLessOrEq    FilterOperator = "lte"      // Less than or equal
	OpIn          FilterOperator = "in"       // In list
	OpNotIn       FilterOperator = "not_in"   // Not in list
	OpRange       FilterOperator = "range"    // Range (min, max)
	OpGeoRadius   FilterOperator = "geo_radius" // Geographic radius
	OpExists      FilterOperator = "exists"   // Field exists
	OpAnd         FilterOperator = "and"      // Logical AND
	OpOr          FilterOperator = "or"       // Logical OR
	OpNot         FilterOperator = "not"      // Logical NOT
)

// ComparisonFilter filters based on field comparison
type ComparisonFilter struct {
	Field    string
	Operator FilterOperator
	Value    interface{}
}

// Match implements Filter interface
func (f *ComparisonFilter) Match(metadata map[string]interface{}) bool {
	fieldValue, exists := metadata[f.Field]
	if !exists {
		return false
	}

	switch f.Operator {
	case OpEquals:
		return equals(fieldValue, f.Value)

	case OpNotEquals:
		return !equals(fieldValue, f.Value)

	case OpGreaterThan:
		return compare(fieldValue, f.Value) > 0

	case OpLessThan:
		return compare(fieldValue, f.Value) < 0

	case OpGreaterOrEq:
		cmp := compare(fieldValue, f.Value)
		return cmp > 0 || cmp == 0

	case OpLessOrEq:
		cmp := compare(fieldValue, f.Value)
		return cmp < 0 || cmp == 0

	case OpExists:
		return exists

	default:
		return false
	}
}

// RangeFilter filters based on numeric range
type RangeFilter struct {
	Field string
	Min   interface{} // Minimum value (inclusive)
	Max   interface{} // Maximum value (inclusive)
}

// Match implements Filter interface
func (f *RangeFilter) Match(metadata map[string]interface{}) bool {
	fieldValue, exists := metadata[f.Field]
	if !exists {
		return false
	}

	// Check if value is within range
	if f.Min != nil && compare(fieldValue, f.Min) < 0 {
		return false
	}

	if f.Max != nil && compare(fieldValue, f.Max) > 0 {
		return false
	}

	return true
}

// InListFilter filters based on whether value is in a list
type InListFilter struct {
	Field  string
	Values []interface{}
	Negate bool // If true, acts as NOT IN
}

// Match implements Filter interface
func (f *InListFilter) Match(metadata map[string]interface{}) bool {
	fieldValue, exists := metadata[f.Field]
	if !exists {
		return f.Negate // If field doesn't exist, NOT IN returns true
	}

	found := false
	for _, v := range f.Values {
		if equals(fieldValue, v) {
			found = true
			break
		}
	}

	if f.Negate {
		return !found
	}
	return found
}

// GeoPoint represents a geographic coordinate
type GeoPoint struct {
	Lat float64 // Latitude
	Lon float64 // Longitude
}

// GeoRadiusFilter filters based on geographic distance
type GeoRadiusFilter struct {
	Field       string   // Field containing GeoPoint
	Center      GeoPoint // Center point
	RadiusKm    float64  // Radius in kilometers
	RadiusMeters float64  // Radius in meters (takes precedence)
}

// Match implements Filter interface
func (f *GeoRadiusFilter) Match(metadata map[string]interface{}) bool {
	fieldValue, exists := metadata[f.Field]
	if !exists {
		return false
	}

	// Extract GeoPoint from field
	var point GeoPoint
	switch v := fieldValue.(type) {
	case GeoPoint:
		point = v
	case map[string]interface{}:
		lat, latOk := v["lat"].(float64)
		lon, lonOk := v["lon"].(float64)
		if !latOk || !lonOk {
			// Try converting from other numeric types
			lat = toFloat64(v["lat"])
			lon = toFloat64(v["lon"])
		}
		point = GeoPoint{Lat: lat, Lon: lon}
	default:
		return false
	}

	// Calculate distance
	distance := haversineDistance(f.Center, point)

	// Check against radius
	radius := f.RadiusMeters
	if radius == 0 {
		radius = f.RadiusKm * 1000 // Convert km to meters
	}

	return distance <= radius
}

// CompositeFilter combines multiple filters with logical operations
type CompositeFilter struct {
	Operator FilterOperator // OpAnd, OpOr, or OpNot
	Filters  []Filter
}

// Match implements Filter interface
func (f *CompositeFilter) Match(metadata map[string]interface{}) bool {
	switch f.Operator {
	case OpAnd:
		for _, filter := range f.Filters {
			if !filter.Match(metadata) {
				return false
			}
		}
		return true

	case OpOr:
		for _, filter := range f.Filters {
			if filter.Match(metadata) {
				return true
			}
		}
		return false

	case OpNot:
		if len(f.Filters) == 0 {
			return true
		}
		return !f.Filters[0].Match(metadata)

	default:
		return false
	}
}

// ExistsFilter checks if a field exists in metadata
type ExistsFilter struct {
	Field  string
	Exists bool // If false, checks that field does NOT exist
}

// Match implements Filter interface
func (f *ExistsFilter) Match(metadata map[string]interface{}) bool {
	_, exists := metadata[f.Field]
	if f.Exists {
		return exists
	}
	return !exists
}

// Helper functions

// equals compares two values for equality
func equals(a, b interface{}) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Direct comparison
	if a == b {
		return true
	}

	// Type-specific comparisons
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok {
			return av == bv
		}
		// Try converting b to int
		return av == int(toFloat64(b))

	case float64:
		return av == toFloat64(b)

	case string:
		if bv, ok := b.(string); ok {
			return av == bv
		}

	case bool:
		if bv, ok := b.(bool); ok {
			return av == bv
		}

	case time.Time:
		if bv, ok := b.(time.Time); ok {
			return av.Equal(bv)
		}
	}

	return false
}

// compare returns -1 if a < b, 0 if a == b, 1 if a > b
func compare(a, b interface{}) int {
	// Handle nil cases
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Numeric comparison
	aNum := toFloat64(a)
	bNum := toFloat64(b)

	if aNum < bNum {
		return -1
	}
	if aNum > bNum {
		return 1
	}
	return 0
}

// toFloat64 converts various numeric types to float64
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	default:
		return 0
	}
}

// haversineDistance calculates the distance between two points on Earth (in meters)
// using the Haversine formula
func haversineDistance(p1, p2 GeoPoint) float64 {
	const earthRadiusMeters = 6371000.0

	// Convert to radians
	lat1 := p1.Lat * math.Pi / 180.0
	lat2 := p2.Lat * math.Pi / 180.0
	lon1 := p1.Lon * math.Pi / 180.0
	lon2 := p2.Lon * math.Pi / 180.0

	// Haversine formula
	dLat := lat2 - lat1
	dLon := lon2 - lon1

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusMeters * c
}

// Builder functions for convenient filter creation

// Eq creates an equality filter
func Eq(field string, value interface{}) Filter {
	return &ComparisonFilter{
		Field:    field,
		Operator: OpEquals,
		Value:    value,
	}
}

// Ne creates a not-equals filter
func Ne(field string, value interface{}) Filter {
	return &ComparisonFilter{
		Field:    field,
		Operator: OpNotEquals,
		Value:    value,
	}
}

// Gt creates a greater-than filter
func Gt(field string, value interface{}) Filter {
	return &ComparisonFilter{
		Field:    field,
		Operator: OpGreaterThan,
		Value:    value,
	}
}

// Lt creates a less-than filter
func Lt(field string, value interface{}) Filter {
	return &ComparisonFilter{
		Field:    field,
		Operator: OpLessThan,
		Value:    value,
	}
}

// Gte creates a greater-than-or-equal filter
func Gte(field string, value interface{}) Filter {
	return &ComparisonFilter{
		Field:    field,
		Operator: OpGreaterOrEq,
		Value:    value,
	}
}

// Lte creates a less-than-or-equal filter
func Lte(field string, value interface{}) Filter {
	return &ComparisonFilter{
		Field:    field,
		Operator: OpLessOrEq,
		Value:    value,
	}
}

// Range creates a range filter
func Range(field string, min, max interface{}) Filter {
	return &RangeFilter{
		Field: field,
		Min:   min,
		Max:   max,
	}
}

// In creates an in-list filter
func In(field string, values ...interface{}) Filter {
	return &InListFilter{
		Field:  field,
		Values: values,
		Negate: false,
	}
}

// NotIn creates a not-in-list filter
func NotIn(field string, values ...interface{}) Filter {
	return &InListFilter{
		Field:  field,
		Values: values,
		Negate: true,
	}
}

// GeoRadius creates a geographic radius filter (radius in kilometers)
func GeoRadius(field string, lat, lon, radiusKm float64) Filter {
	return &GeoRadiusFilter{
		Field:    field,
		Center:   GeoPoint{Lat: lat, Lon: lon},
		RadiusKm: radiusKm,
	}
}

// GeoRadiusMeters creates a geographic radius filter (radius in meters)
func GeoRadiusMeters(field string, lat, lon, radiusMeters float64) Filter {
	return &GeoRadiusFilter{
		Field:        field,
		Center:       GeoPoint{Lat: lat, Lon: lon},
		RadiusMeters: radiusMeters,
	}
}

// Exists creates an exists filter
func Exists(field string) Filter {
	return &ExistsFilter{
		Field:  field,
		Exists: true,
	}
}

// NotExists creates a not-exists filter
func NotExists(field string) Filter {
	return &ExistsFilter{
		Field:  field,
		Exists: false,
	}
}

// And creates a composite AND filter
func And(filters ...Filter) Filter {
	return &CompositeFilter{
		Operator: OpAnd,
		Filters:  filters,
	}
}

// Or creates a composite OR filter
func Or(filters ...Filter) Filter {
	return &CompositeFilter{
		Operator: OpOr,
		Filters:  filters,
	}
}

// Not creates a composite NOT filter
func Not(filter Filter) Filter {
	return &CompositeFilter{
		Operator: OpNot,
		Filters:  []Filter{filter},
	}
}

// FilterBuilder provides a fluent interface for building complex filters
type FilterBuilder struct {
	filter Filter
	err    error
}

// NewFilterBuilder creates a new filter builder
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{}
}

// Equals adds an equality condition
func (fb *FilterBuilder) Equals(field string, value interface{}) *FilterBuilder {
	fb.filter = Eq(field, value)
	return fb
}

// GreaterThan adds a greater-than condition
func (fb *FilterBuilder) GreaterThan(field string, value interface{}) *FilterBuilder {
	fb.filter = Gt(field, value)
	return fb
}

// Build returns the constructed filter
func (fb *FilterBuilder) Build() (Filter, error) {
	if fb.err != nil {
		return nil, fb.err
	}
	if fb.filter == nil {
		return nil, fmt.Errorf("no filter conditions specified")
	}
	return fb.filter, nil
}
