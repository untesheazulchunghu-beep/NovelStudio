package app

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
)

func (s *Store) refreshWorkspaceIfChangedLocked(workspaceID string) error {
	var updatedAt string
	err := s.db.QueryRow(`SELECT updated_at FROM workspaces WHERE workspace_id = ?`, workspaceID).Scan(&updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		delete(s.workspaces, workspaceID)
		return os.ErrNotExist
	}
	if err != nil {
		return fmt.Errorf("check workspace freshness %s: %w", workspaceID, err)
	}
	if workspace, exists := s.workspaces[workspaceID]; exists && workspace.Project.UpdatedAt == updatedAt {
		return nil
	}
	return s.refreshWorkspaceLocked(workspaceID)
}

func (s *Store) refreshWorkspaceLocked(workspaceID string) error {
	workspace, err := loadWorkspaceMetadata(s.db, workspaceID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			delete(s.workspaces, workspaceID)
			return os.ErrNotExist
		}
		return fmt.Errorf("refresh workspace %s: %w", workspaceID, err)
	}
	workspace.ID = workspaceID
	normalizeWorkspaceMetadata(&workspace)
	s.workspaces[workspaceID] = workspace
	return nil
}

func (s *Store) nextIDLocked() int64 {
	max := int64(0)
	for _, workspace := range s.workspaces {
		workspaceMax := maxID(workspace)
		if workspaceMax > max {
			max = workspaceMax
		}
	}
	return max + 1
}

func maxID(workspace Workspace) int64 {
	max := workspace.Project.ID
	for _, chapter := range workspace.Chapters {
		if chapter.ID > max {
			max = chapter.ID
		}
	}
	for _, character := range workspace.Characters {
		if character.ID > max {
			max = character.ID
		}
	}
	for _, note := range workspace.Notes {
		if note.ID > max {
			max = note.ID
		}
	}
	for _, message := range workspace.Chat {
		if message.ID > max {
			max = message.ID
		}
	}
	return max
}

func touchProject(workspace *Workspace) {
	workspace.Project.UpdatedAt = now()
	workspace.UpdatedAt = workspace.Project.UpdatedAt
}

func (s *Store) persistWorkspaceLocked(workspaceID string, workspace Workspace) error {
	workspace.ID = workspaceID
	workspace.UpdatedAt = now()
	workspace.Project.UpdatedAt = workspace.UpdatedAt
	var err error
	if previous, exists := s.workspaces[workspaceID]; exists {
		err = persistWorkspaceDelta(s.db, workspaceID, previous, workspace)
	} else {
		err = persistWorkspace(s.db, workspaceID, workspace)
	}
	if err != nil {
		return err
	}
	s.workspaces[workspaceID] = workspaceMetadata(workspace)
	return nil
}

func workspaceMetadata(workspace Workspace) Workspace {
	metadata := cloneWorkspace(workspace)
	for index := range metadata.Chapters {
		metadata.Chapters[index].Content = ""
	}
	return metadata
}

func seedWorkspace() Workspace {
	created := now()
	return Workspace{
		ID:      localWorkspaceID,
		Project: Project{ID: 1, Title: "星火回廊", Subtitle: "一封来自未来的信", Genre: "近未来 · 悬疑", Premise: "在记忆可以被备份的雾港，失踪的姐姐寄来一封尚未发生的信。", Author: "我的工作区", Status: "draft", CreatedAt: created, UpdatedAt: created, WordTarget: 120000, CurrentView: "editor"},
		Chapters: []Chapter{
			{ID: 2, Number: 1, Title: "雾港来信", Summary: "林澈在凌晨收到姐姐寄来的信，信上写着一场尚未发生的事故。", Content: "凌晨四点十七分，雾港的潮声像一台没有关掉的旧收音机。\n\n林澈在门缝里发现那只信封时，先看见的是邮戳。日期是三天以后。\n\n她把信翻过来，封口处没有胶水，只有一枚暗银色的回廊印记。那是姐姐林栀失踪前，留在桌上的最后一样东西。\n\n“不要让钟楼在第七声之后响。”\n\n纸上的字迹很轻，像写信的人正站在很远的地方。", Status: "draft", LastEditedAt: created},
			{ID: 3, Number: 2, Title: "第七声钟响", Summary: "林澈追着信中的线索来到旧钟楼，发现时间记录被人为改写。", Content: "旧钟楼的门没有锁。\n\n林澈推开门，灰尘从门楣落下，露出墙上密密麻麻的刻痕。每一道刻痕旁都写着一个时间，唯独七点整的位置被人刮去了。", Status: "outline", LastEditedAt: created},
			{ID: 4, Number: 3, Title: "回廊尽头", Summary: "一条只在潮汐最低时出现的走廊，通向姐姐留下的真相。", Content: "", Status: "outline", LastEditedAt: created},
		},
		Characters: []Character{
			{ID: 5, Name: "林澈", Role: "主角 · 记忆修复师", Description: "习惯先修复别人的记忆，再处理自己的情绪。", Traits: []string{"敏锐", "克制", "怕水"}, Color: "#f0a46b"},
			{ID: 6, Name: "林栀", Role: "失踪的姐姐", Description: "留下预言般书信的量子考古学家。", Traits: []string{"理性", "冒险", "执拗"}, Color: "#8f9df4"},
			{ID: 7, Name: "周既明", Role: "钟楼管理员", Description: "知道雾港每一座钟的秘密，却从不谈论第七声。", Traits: []string{"温和", "多疑", "守旧"}, Color: "#58c5a3"},
		},
		Notes: []Note{
			{ID: 8, Title: "核心悬念", Body: "姐姐寄来的信来自三天以后；第七声钟响会让一段记忆被覆盖。", Kind: "mystery", UpdatedAt: created},
			{ID: 9, Title: "写作提醒", Body: "让每章都出现一种与潮汐有关的感官意象。", Kind: "craft", UpdatedAt: created},
		},
		Settings:  Settings{Provider: "local", Model: "local-creative", Temperature: 0.7},
		UpdatedAt: created,
	}
}

func calculateStats(workspace Workspace) Stats {
	stats := Stats{WordTarget: workspace.Project.WordTarget, ChapterCount: len(workspace.Chapters)}
	for _, chapter := range workspace.Chapters {
		stats.WordCount += chapter.WordCount
		if chapter.Status == "done" {
			stats.FinishedCount++
		}
	}
	if stats.WordTarget > 0 {
		stats.CompletionRate = float64(stats.WordCount) / float64(stats.WordTarget)
	}
	if stats.CompletionRate > 1 {
		stats.CompletionRate = 1
	}
	return stats
}

func cloneWorkspace(input Workspace) Workspace {
	output := input
	output.Chapters = append([]Chapter{}, input.Chapters...)
	output.Characters = make([]Character, len(input.Characters))
	for index, character := range input.Characters {
		output.Characters[index] = characterCopy(character)
	}
	output.Notes = append([]Note{}, input.Notes...)
	output.Chat = append([]ChatMessage{}, input.Chat...)
	output.Settings = input.Settings
	return output
}

func chapterCopy(chapter Chapter) Chapter { return chapter }

func characterCopy(character Character) Character {
	character.Traits = append([]string{}, character.Traits...)
	return character
}
