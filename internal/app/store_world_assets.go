package app

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func (s *Store) CreateWorldAsset(workspaceID string, input WorldAssetInput) (WorldAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return WorldAsset{}, os.ErrNotExist
	}
	asset, err := normalizeWorldAssetInput(workspace, input, 0)
	if err != nil {
		return WorldAsset{}, err
	}
	workspace = cloneWorkspace(workspace)
	asset.ID = s.nextIDLocked()
	asset.UpdatedAt = now()
	workspace.WorldAssets = append(workspace.WorldAssets, asset)
	touchProject(&workspace)
	if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
		return WorldAsset{}, err
	}
	return worldAssetCopy(asset), nil
}

func (s *Store) UpdateWorldAsset(workspaceID string, id int64, input WorldAssetInput) (WorldAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return WorldAsset{}, os.ErrNotExist
	}
	asset, err := normalizeWorldAssetInput(workspace, input, id)
	if err != nil {
		return WorldAsset{}, err
	}
	workspace = cloneWorkspace(workspace)
	for index := range workspace.WorldAssets {
		if workspace.WorldAssets[index].ID != id {
			continue
		}
		asset.ID = id
		asset.UpdatedAt = now()
		workspace.WorldAssets[index] = asset
		touchProject(&workspace)
		if err := s.persistWorkspaceLocked(workspaceID, workspace); err != nil {
			return WorldAsset{}, err
		}
		return worldAssetCopy(asset), nil
	}
	return WorldAsset{}, os.ErrNotExist
}

func (s *Store) DeleteWorldAsset(workspaceID string, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, exists := s.workspaces[workspaceID]
	if !exists {
		return os.ErrNotExist
	}
	workspace = cloneWorkspace(workspace)
	for index, asset := range workspace.WorldAssets {
		if asset.ID != id {
			continue
		}
		workspace.WorldAssets = append(workspace.WorldAssets[:index], workspace.WorldAssets[index+1:]...)
		for assetIndex := range workspace.WorldAssets {
			workspace.WorldAssets[assetIndex].RelatedAssetIDs = removeID(workspace.WorldAssets[assetIndex].RelatedAssetIDs, id)
		}
		touchProject(&workspace)
		return s.persistWorkspaceLocked(workspaceID, workspace)
	}
	return os.ErrNotExist
}

func normalizeWorldAssetInput(workspace Workspace, input WorldAssetInput, selfID int64) (WorldAsset, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return WorldAsset{}, fmt.Errorf("%w: world asset name is required", ErrInvalidWorldAssetInput)
	}
	if err := validateWorldAssetTextFields(input); err != nil {
		return WorldAsset{}, fmt.Errorf("%w: %v", ErrInvalidWorldAssetInput, err)
	}
	kind, err := normalizeWorldAssetKind(input.Kind)
	if err != nil {
		return WorldAsset{}, fmt.Errorf("%w: %v", ErrInvalidWorldAssetInput, err)
	}
	chapters, err := normalizeReferencedIDs(input.ChapterIDs, func(id int64) bool {
		_, found := findChapter(workspace, id)
		return found
	})
	if err != nil {
		return WorldAsset{}, fmt.Errorf("%w: %v", ErrInvalidWorldAssetInput, err)
	}
	assetsByID := make(map[int64]struct{}, len(workspace.WorldAssets))
	for _, asset := range workspace.WorldAssets {
		assetsByID[asset.ID] = struct{}{}
	}
	related, err := normalizeReferencedIDs(input.RelatedAssetIDs, func(id int64) bool {
		_, found := assetsByID[id]
		return found && id != selfID
	})
	if err != nil {
		return WorldAsset{}, fmt.Errorf("%w: %v", ErrInvalidWorldAssetInput, err)
	}
	return WorldAsset{
		Name: name, Kind: kind, Description: strings.TrimSpace(input.Description), Details: strings.TrimSpace(input.Details),
		ChapterIDs: chapters, RelatedAssetIDs: related,
	}, nil
}

func normalizeWorldAssetKind(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "place":
		return "place", nil
	case "organization":
		return "organization", nil
	case "item":
		return "item", nil
	case "concept":
		return "concept", nil
	default:
		return "", fmt.Errorf("world asset kind must be place, organization, item, or concept")
	}
}

func normalizeReferencedIDs(ids []int64, valid func(int64) bool) ([]int64, error) {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("referenced id must be positive")
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		if !valid(id) {
			return nil, fmt.Errorf("referenced id %d was not found", id)
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result, nil
}

func removeID(ids []int64, target int64) []int64 {
	result := ids[:0]
	for _, id := range ids {
		if id != target {
			result = append(result, id)
		}
	}
	return result
}

func worldAssetCopy(asset WorldAsset) WorldAsset {
	asset.ChapterIDs = append([]int64{}, asset.ChapterIDs...)
	asset.RelatedAssetIDs = append([]int64{}, asset.RelatedAssetIDs...)
	return asset
}
