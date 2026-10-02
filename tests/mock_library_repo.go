package tests

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"

	"github.com/Masterminds/squirrel"
	"github.com/deluan/rest"
	"github.com/navidrome/navidrome/model"
)

// MockLibraryRepo 用内存 map 模拟 library 表。core.NewLibrary 会起后台 goroutine
// （autoDetectMediaType）并发读库，真实实现是 DB（本身线程安全），mock 也必须是
// 线程安全的——否则测试会随机撞上 "concurrent map iteration and map write"。
type MockLibraryRepo struct {
	model.LibraryRepository
	mu    sync.RWMutex
	Data  map[int]model.Library
	Err   error
	PutFn func(*model.Library) error // Allow custom Put behavior for testing
}

func (m *MockLibraryRepo) SetData(data model.Libraries) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Data = make(map[int]model.Library)
	for _, d := range data {
		m.Data[d.ID] = d
	}
}

func (m *MockLibraryRepo) GetAll(...model.QueryOptions) (model.Libraries, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Err != nil {
		return nil, m.Err
	}
	var libraries model.Libraries
	for _, lib := range m.Data {
		libraries = append(libraries, lib)
	}
	// Sort by ID for predictable order
	slices.SortFunc(libraries, func(a, b model.Library) int {
		return a.ID - b.ID
	})
	return libraries, nil
}

func (m *MockLibraryRepo) countAllLocked(qo ...model.QueryOptions) (int64, error) {
	if m.Err != nil {
		return 0, m.Err
	}

	// If no query options, return total count
	if len(qo) == 0 || qo[0].Filters == nil {
		return int64(len(m.Data)), nil
	}

	// Handle squirrel.Eq filter for ID validation
	if eq, ok := qo[0].Filters.(squirrel.Eq); ok {
		if idFilter, exists := eq["id"]; exists {
			if ids, isSlice := idFilter.([]int); isSlice {
				count := 0
				for _, id := range ids {
					if _, exists := m.Data[id]; exists {
						count++
					}
				}
				return int64(count), nil
			}
		}
	}

	// Default to total count for other filters
	return int64(len(m.Data)), nil
}

func (m *MockLibraryRepo) CountAll(qo ...model.QueryOptions) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.countAllLocked(qo...)
}

func (m *MockLibraryRepo) getLocked(id int) (*model.Library, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if lib, ok := m.Data[id]; ok {
		return &lib, nil
	}
	return nil, model.ErrNotFound
}

func (m *MockLibraryRepo) Get(id int) (*model.Library, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getLocked(id)
}

func (m *MockLibraryRepo) GetPath(id int) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Err != nil {
		return "", m.Err
	}
	if lib, ok := m.Data[id]; ok {
		return lib.Path, nil
	}
	return "", model.ErrNotFound
}

func (m *MockLibraryRepo) Put(library *model.Library) error {
	if m.PutFn != nil {
		return m.PutFn(library)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if m.Data == nil {
		m.Data = make(map[int]model.Library)
	}
	m.Data[library.ID] = *library
	return nil
}

func (m *MockLibraryRepo) Delete(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if _, ok := m.Data[id]; !ok {
		return model.ErrNotFound
	}
	delete(m.Data, id)
	return nil
}

func (m *MockLibraryRepo) StoreMusicFolder() error {
	if m.Err != nil {
		return m.Err
	}
	return nil
}

func (m *MockLibraryRepo) AddArtist(id int, artistID string) error {
	if m.Err != nil {
		return m.Err
	}
	return nil
}

func (m *MockLibraryRepo) ScanBegin(id int, fullScan bool) error {
	if m.Err != nil {
		return m.Err
	}
	return nil
}

func (m *MockLibraryRepo) ScanEnd(id int) error {
	if m.Err != nil {
		return m.Err
	}
	return nil
}

func (m *MockLibraryRepo) ScanInProgress() (bool, error) {
	if m.Err != nil {
		return false, m.Err
	}
	return false, nil
}

func (m *MockLibraryRepo) RefreshStats(id int) error {
	return nil
}

// User-library association methods - mock implementations

func (m *MockLibraryRepo) GetUsersWithLibraryAccess(libraryID int) (model.Users, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	// Mock: return empty users for now
	return model.Users{}, nil
}

func (m *MockLibraryRepo) Count(options ...rest.QueryOptions) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.countAllLocked()
}

func (m *MockLibraryRepo) Read(id string) (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	idInt, _ := strconv.Atoi(id)
	mf, err := m.getLocked(idInt)
	if errors.Is(err, model.ErrNotFound) {
		return nil, rest.ErrNotFound
	}
	return mf, err
}

func (m *MockLibraryRepo) ReadAll(options ...rest.QueryOptions) (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Err != nil {
		return nil, m.Err
	}
	var libraries model.Libraries
	for _, lib := range m.Data {
		libraries = append(libraries, lib)
	}
	slices.SortFunc(libraries, func(a, b model.Library) int {
		return a.ID - b.ID
	})
	return libraries, nil
}

func (m *MockLibraryRepo) EntityName() string {
	return "library"
}

func (m *MockLibraryRepo) NewInstance() any {
	return &model.Library{}
}

// REST Repository methods (string-based IDs)

func (m *MockLibraryRepo) Save(entity any) (string, error) {
	lib := entity.(*model.Library)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return "", m.Err
	}

	// Validate required fields
	if lib.Name == "" {
		return "", &rest.ValidationError{Errors: map[string]string{"name": "library name is required"}}
	}
	if lib.Path == "" {
		return "", &rest.ValidationError{Errors: map[string]string{"path": "library path is required"}}
	}

	// Generate ID if not set
	if lib.ID == 0 {
		lib.ID = len(m.Data) + 1
	}
	if m.Data == nil {
		m.Data = make(map[int]model.Library)
	}
	m.Data[lib.ID] = *lib
	return strconv.Itoa(lib.ID), nil
}

func (m *MockLibraryRepo) Update(id string, entity any, cols ...string) error {
	lib := entity.(*model.Library)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}

	// Validate required fields
	if lib.Name == "" {
		return &rest.ValidationError{Errors: map[string]string{"name": "library name is required"}}
	}
	if lib.Path == "" {
		return &rest.ValidationError{Errors: map[string]string{"path": "library path is required"}}
	}

	idInt, err := strconv.Atoi(id)
	if err != nil {
		return errors.New("invalid ID format")
	}
	if _, exists := m.Data[idInt]; !exists {
		return rest.ErrNotFound
	}
	lib.ID = idInt
	m.Data[idInt] = *lib
	return nil
}

func (m *MockLibraryRepo) DeleteByStringID(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return errors.New("invalid ID format")
	}
	if _, exists := m.Data[idInt]; !exists {
		return rest.ErrNotFound
	}
	delete(m.Data, idInt)
	return nil
}

// Service-level methods for core.Library interface

func (m *MockLibraryRepo) GetUserLibraries(ctx context.Context, userID string) (model.Libraries, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Err != nil {
		return nil, m.Err
	}
	if userID == "non-existent" {
		return nil, model.ErrNotFound
	}
	// Convert map to slice for return
	var libraries model.Libraries
	for _, lib := range m.Data {
		libraries = append(libraries, lib)
	}
	// Sort by ID for predictable order
	slices.SortFunc(libraries, func(a, b model.Library) int {
		return a.ID - b.ID
	})
	return libraries, nil
}

func (m *MockLibraryRepo) SetUserLibraries(ctx context.Context, userID string, libraryIDs []int) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.Err != nil {
		return m.Err
	}
	if userID == "non-existent" {
		return model.ErrNotFound
	}
	if userID == "admin-1" {
		return fmt.Errorf("%w: cannot manually assign libraries to admin users", model.ErrValidation)
	}
	if len(libraryIDs) == 0 {
		return fmt.Errorf("%w: at least one library must be assigned to non-admin users", model.ErrValidation)
	}
	// Validate all library IDs exist
	for _, id := range libraryIDs {
		if _, exists := m.Data[id]; !exists {
			return fmt.Errorf("%w: library ID %d does not exist", model.ErrValidation, id)
		}
	}
	return nil
}

func (m *MockLibraryRepo) ValidateLibraryAccess(ctx context.Context, userID string, libraryID int) error {
	if m.Err != nil {
		return m.Err
	}
	// For testing purposes, allow access to all libraries
	return nil
}

var _ model.LibraryRepository = (*MockLibraryRepo)(nil)
var _ model.ResourceRepository = (*MockLibraryRepo)(nil)
