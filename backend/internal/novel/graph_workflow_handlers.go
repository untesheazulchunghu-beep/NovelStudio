package novel

import (
	"encoding/json"
	"net/http"
	"strings"

	"novelstudio/internal/database"
)

type GraphNode struct {
	ID      string  `json:"id"`
	NovelID string  `json:"novelId"`
	Kind    string  `json:"kind"`
	Label   string  `json:"label"`
	Summary string  `json:"summary"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
}

type GraphEdge struct {
	ID       string `json:"id"`
	NovelID  string `json:"novelId"`
	SourceID string `json:"sourceId"`
	TargetID string `json:"targetId"`
	Relation string `json:"relation"`
	Summary  string `json:"summary"`
}

func (s *Server) getGraph(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	novelID := r.PathValue("novelID")
	nodeRows, err := s.db.QueryContext(r.Context(), `SELECT id, novel_id, kind, label, summary, x, y FROM graph_nodes WHERE novel_id = ? ORDER BY created_at`, novelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取知识图谱失败")
		return
	}
	defer nodeRows.Close()
	nodes := make([]GraphNode, 0)
	for nodeRows.Next() {
		var node GraphNode
		if err := nodeRows.Scan(&node.ID, &node.NovelID, &node.Kind, &node.Label, &node.Summary, &node.X, &node.Y); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取知识图谱失败")
			return
		}
		nodes = append(nodes, node)
	}
	if err := nodeRows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取知识图谱失败")
		return
	}
	edgeRows, err := s.db.QueryContext(r.Context(), `SELECT id, novel_id, source_id, target_id, relation, summary FROM graph_edges WHERE novel_id = ? ORDER BY created_at`, novelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取关系失败")
		return
	}
	defer edgeRows.Close()
	edges := make([]GraphEdge, 0)
	for edgeRows.Next() {
		var edge GraphEdge
		if err := edgeRows.Scan(&edge.ID, &edge.NovelID, &edge.SourceID, &edge.TargetID, &edge.Relation, &edge.Summary); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取关系失败")
			return
		}
		edges = append(edges, edge)
	}
	if err := edgeRows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取关系失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes, "edges": edges})
}

func (s *Server) createGraphNode(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	var input struct {
		Kind    string  `json:"kind"`
		Label   string  `json:"label"`
		Summary string  `json:"summary"`
		X       float64 `json:"x"`
		Y       float64 `json:"y"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Kind = strings.TrimSpace(input.Kind)
	input.Label = strings.TrimSpace(input.Label)
	if !validNodeKind(input.Kind) || input.Label == "" || len([]rune(input.Label)) > 80 {
		writeError(w, http.StatusBadRequest, "INVALID_NODE", "实体类型无效，名称不能为空且不超过 80 字")
		return
	}
	node := GraphNode{ID: database.NewID(), NovelID: r.PathValue("novelID"), Kind: input.Kind, Label: input.Label, Summary: strings.TrimSpace(input.Summary), X: input.X, Y: input.Y}
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO graph_nodes (id, novel_id, kind, label, summary, x, y, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, node.ID, node.NovelID, node.Kind, node.Label, node.Summary, node.X, node.Y, database.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建知识实体失败")
		return
	}
	writeJSON(w, http.StatusCreated, node)
}

func (s *Server) createGraphEdge(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	var input struct {
		SourceID string `json:"sourceId"`
		TargetID string `json:"targetId"`
		Relation string `json:"relation"`
		Summary  string `json:"summary"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Relation = strings.TrimSpace(input.Relation)
	if input.SourceID == "" || input.TargetID == "" || input.SourceID == input.TargetID || input.Relation == "" || len([]rune(input.Relation)) > 80 {
		writeError(w, http.StatusBadRequest, "INVALID_EDGE", "关系两端必须是不同实体，关系名称不能为空且不超过 80 字")
		return
	}
	edge := GraphEdge{ID: database.NewID(), NovelID: r.PathValue("novelID"), SourceID: input.SourceID, TargetID: input.TargetID, Relation: input.Relation, Summary: strings.TrimSpace(input.Summary)}
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO graph_edges (id, novel_id, source_id, target_id, relation, summary, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, edge.ID, edge.NovelID, edge.SourceID, edge.TargetID, edge.Relation, edge.Summary, database.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_EDGE", "关系指向的实体不存在或不属于当前小说")
		return
	}
	writeJSON(w, http.StatusCreated, edge)
}

func (s *Server) updateGraphNode(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	var input struct {
		Kind    string  `json:"kind"`
		Label   string  `json:"label"`
		Summary string  `json:"summary"`
		X       float64 `json:"x"`
		Y       float64 `json:"y"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Kind, input.Label, input.Summary = strings.TrimSpace(input.Kind), strings.TrimSpace(input.Label), strings.TrimSpace(input.Summary)
	if !validNodeKind(input.Kind) || input.Label == "" || len([]rune(input.Label)) > 80 {
		writeError(w, http.StatusBadRequest, "INVALID_NODE", "实体类型无效，名称不能为空且不超过 80 字")
		return
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE graph_nodes SET kind = ?, label = ?, summary = ?, x = ?, y = ? WHERE id = ? AND novel_id = ?`, input.Kind, input.Label, input.Summary, input.X, input.Y, r.PathValue("nodeID"), r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "更新知识实体失败")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, http.StatusNotFound, "NODE_NOT_FOUND", "知识实体不存在")
		return
	}
	var node GraphNode
	err = s.db.QueryRowContext(r.Context(), `SELECT id, novel_id, kind, label, summary, x, y FROM graph_nodes WHERE id = ? AND novel_id = ?`, r.PathValue("nodeID"), r.PathValue("novelID")).Scan(&node.ID, &node.NovelID, &node.Kind, &node.Label, &node.Summary, &node.X, &node.Y)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取知识实体失败")
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (s *Server) deleteGraphNode(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM graph_nodes WHERE id = ? AND novel_id = ?`, r.PathValue("nodeID"), r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "删除知识实体失败")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, http.StatusNotFound, "NODE_NOT_FOUND", "知识实体不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) updateGraphEdge(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	var input struct {
		SourceID string `json:"sourceId"`
		TargetID string `json:"targetId"`
		Relation string `json:"relation"`
		Summary  string `json:"summary"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Relation, input.Summary = strings.TrimSpace(input.Relation), strings.TrimSpace(input.Summary)
	if input.SourceID == "" || input.TargetID == "" || input.SourceID == input.TargetID || input.Relation == "" || len([]rune(input.Relation)) > 80 {
		writeError(w, http.StatusBadRequest, "INVALID_EDGE", "关系两端必须是不同实体，关系名称不能为空且不超过 80 字")
		return
	}
	result, err := s.db.ExecContext(r.Context(), `UPDATE graph_edges SET source_id = ?, target_id = ?, relation = ?, summary = ? WHERE id = ? AND novel_id = ? AND EXISTS (SELECT 1 FROM graph_nodes WHERE id = ? AND novel_id = ?) AND EXISTS (SELECT 1 FROM graph_nodes WHERE id = ? AND novel_id = ?)`, input.SourceID, input.TargetID, input.Relation, input.Summary, r.PathValue("edgeID"), r.PathValue("novelID"), input.SourceID, r.PathValue("novelID"), input.TargetID, r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_EDGE", "关系指向的实体不存在或不属于当前小说")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, http.StatusNotFound, "EDGE_NOT_FOUND", "关系不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) deleteGraphEdge(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM graph_edges WHERE id = ? AND novel_id = ?`, r.PathValue("edgeID"), r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "删除关系失败")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, http.StatusNotFound, "EDGE_NOT_FOUND", "关系不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func validNodeKind(kind string) bool {
	return kind == "character" || kind == "place" || kind == "faction" || kind == "item" || kind == "concept"
}

type Workflow struct {
	ID         string          `json:"id"`
	NovelID    string          `json:"novelId"`
	Name       string          `json:"name"`
	Definition json.RawMessage `json:"definition"`
	CreatedAt  string          `json:"createdAt"`
	UpdatedAt  string          `json:"updatedAt"`
}

type WorkflowNode struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	Title  string  `json:"title"`
	Prompt string  `json:"prompt"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}

type WorkflowEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type WorkflowDefinition struct {
	Nodes []WorkflowNode `json:"nodes"`
	Edges []WorkflowEdge `json:"edges"`
}

func (s *Server) listWorkflows(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor", "reviewer", "viewer") {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, novel_id, name, definition_json, created_at, updated_at FROM workflows WHERE novel_id = ? ORDER BY updated_at DESC`, r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取工作流失败")
		return
	}
	defer rows.Close()
	workflows := make([]Workflow, 0)
	for rows.Next() {
		var item Workflow
		var definition string
		if err := rows.Scan(&item.ID, &item.NovelID, &item.Name, &definition, &item.CreatedAt, &item.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取工作流失败")
			return
		}
		item.Definition = json.RawMessage(definition)
		workflows = append(workflows, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "读取工作流失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": workflows})
}

func (s *Server) createWorkflow(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	var input struct {
		Name       string             `json:"name"`
		Definition WorkflowDefinition `json:"definition"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 80 || !validateWorkflow(input.Definition) {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW", "工作流名称或节点连线无效")
		return
	}
	definition, _ := json.Marshal(input.Definition)
	now := database.Now()
	workflow := Workflow{ID: database.NewID(), NovelID: r.PathValue("novelID"), Name: input.Name, Definition: definition, CreatedAt: now, UpdatedAt: now}
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO workflows (id, novel_id, name, definition_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, workflow.ID, workflow.NovelID, workflow.Name, string(definition), now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "创建工作流失败")
		return
	}
	writeJSON(w, http.StatusCreated, workflow)
}

func (s *Server) updateWorkflow(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireNovelRole(w, r.PathValue("novelID"), userID, "owner", "editor") {
		return
	}
	var input struct {
		Name       string             `json:"name"`
		Definition WorkflowDefinition `json:"definition"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 80 || !validateWorkflow(input.Definition) {
		writeError(w, http.StatusBadRequest, "INVALID_WORKFLOW", "工作流名称或节点连线无效")
		return
	}
	definition, _ := json.Marshal(input.Definition)
	result, err := s.db.ExecContext(r.Context(), `UPDATE workflows SET name = ?, definition_json = ?, updated_at = ? WHERE id = ? AND novel_id = ?`, input.Name, string(definition), database.Now(), r.PathValue("workflowID"), r.PathValue("novelID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "保存工作流失败")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		writeError(w, http.StatusNotFound, "WORKFLOW_NOT_FOUND", "工作流不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) deleteWorkflow(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	novelID, workflowID := r.PathValue("novelID"), r.PathValue("workflowID")
	if !ok || !s.requireNovelRole(w, novelID, userID, "owner", "editor") {
		return
	}
	var activeTasks int
	if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM tasks WHERE workflow_id = ? AND status IN ('queued', 'running')`, workflowID).Scan(&activeTasks); err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "检查工作流任务失败")
		return
	}
	if activeTasks > 0 {
		writeError(w, http.StatusConflict, "TASKS_RUNNING", "请等待该工作流的生成任务完成后再删除")
		return
	}
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM workflows WHERE id = ? AND novel_id = ?`, workflowID, novelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DATABASE_ERROR", "删除工作流失败")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		writeError(w, http.StatusNotFound, "WORKFLOW_NOT_FOUND", "工作流不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func validateWorkflow(definition WorkflowDefinition) bool {
	if len(definition.Nodes) == 0 || len(definition.Nodes) > 30 || len(definition.Edges) > 60 {
		return false
	}
	nodes := make(map[string]bool, len(definition.Nodes))
	for _, node := range definition.Nodes {
		if node.ID == "" || nodes[node.ID] || len([]rune(node.Title)) > 100 || len([]rune(node.Prompt)) > 5000 || (node.Kind != "context" && node.Kind != "generate" && node.Kind != "review" && node.Kind != "output") {
			return false
		}
		nodes[node.ID] = true
	}
	edgeIDs := make(map[string]bool, len(definition.Edges))
	for _, edge := range definition.Edges {
		if edge.ID == "" || edgeIDs[edge.ID] || edge.Source == edge.Target || !nodes[edge.Source] || !nodes[edge.Target] {
			return false
		}
		edgeIDs[edge.ID] = true
	}
	return len(orderedWorkflowNodes(definition)) == len(definition.Nodes)
}

func (s *Server) workflowPrompt(r *http.Request, workflowID, novelID string) (string, string, bool) {
	var name, raw string
	if err := s.db.QueryRowContext(r.Context(), `SELECT name, definition_json FROM workflows WHERE id = ? AND novel_id = ?`, workflowID, novelID).Scan(&name, &raw); err != nil {
		return "", "", false
	}
	var definition WorkflowDefinition
	if json.Unmarshal([]byte(raw), &definition) != nil {
		return "", "", false
	}
	ordered := orderedWorkflowNodes(definition)
	if len(ordered) != len(definition.Nodes) {
		return "", "", false
	}
	var instructions strings.Builder
	for _, node := range ordered {
		switch node.Kind {
		case "context":
			instructions.WriteString("\n资料节点「" + node.Title + "」：\n" + node.Prompt + "\n")
		case "generate":
			instructions.WriteString("\n创作节点「" + node.Title + "」：\n" + node.Prompt + "\n")
		case "review":
			instructions.WriteString("\n自检节点「" + node.Title + "」：\n" + node.Prompt + "\n")
		}
	}
	return name, instructions.String(), true
}

func orderedWorkflowNodes(definition WorkflowDefinition) []WorkflowNode {
	byID := make(map[string]WorkflowNode, len(definition.Nodes))
	indegree := make(map[string]int, len(definition.Nodes))
	outgoing := make(map[string][]string, len(definition.Nodes))
	for _, node := range definition.Nodes {
		byID[node.ID] = node
		indegree[node.ID] = 0
	}
	for _, edge := range definition.Edges {
		outgoing[edge.Source] = append(outgoing[edge.Source], edge.Target)
		indegree[edge.Target]++
	}
	queue := make([]string, 0, len(definition.Nodes))
	for _, node := range definition.Nodes {
		if indegree[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}
	ordered := make([]WorkflowNode, 0, len(definition.Nodes))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		ordered = append(ordered, byID[id])
		for _, target := range outgoing[id] {
			indegree[target]--
			if indegree[target] == 0 {
				queue = append(queue, target)
			}
		}
	}
	if len(ordered) != len(definition.Nodes) {
		return nil
	}
	return ordered
}

func (s *Server) workflowExists(workflowID, novelID string) bool {
	var exists bool
	return s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM workflows WHERE id = ? AND novel_id = ?)`, workflowID, novelID).Scan(&exists) == nil && exists
}
