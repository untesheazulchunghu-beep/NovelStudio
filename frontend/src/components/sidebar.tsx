import { api } from '../api'
import { TimelineView } from './timeline'
import { WorldAssetsView } from './worldAssets'
import { statusClass } from '../lib/format'
import type { EditorPreferences } from '../lib/editorPreferences'
import { Archive, ArrowDownToLine, BookOpen, Check, ChevronDown, ChevronUp, CircleHelp, FilePlus2, FileText, FolderOpen, Lightbulb, ListTodo, Pencil, Plus, Save, Settings2, Trash2, Upload, Users } from 'lucide-react'
import type { Chapter, Character, Note, SettingsDraft, WorkspaceResponse } from '../types'
import type { ActivityId } from '../workbenchTypes'

export function ActivityButton({ icon, active, expanded, label, badge, unread, buttonRef, onClick }: { icon: React.ReactNode; active: boolean; expanded?: boolean; label: string; badge?: number; unread?: boolean; buttonRef?: React.Ref<HTMLButtonElement>; onClick: () => void }) {
  const accessibleLabel = badge === undefined ? (unread ? `${label}（有未读 Agent 回复）` : label) : `${label}（${badge}）${unread ? '，有未读 Agent 回复' : ''}`
  return <button ref={buttonRef} className={`activity-button ${active ? 'active' : ''}`} onClick={onClick} title={accessibleLabel} aria-label={accessibleLabel} aria-pressed={expanded === undefined ? active : undefined} aria-expanded={expanded}>{icon}{unread && <span className="activity-unread-dot" aria-hidden="true" />}{badge !== undefined && <span className="activity-badge">{badge}</span>}</button>
}

export function ExplorerView({ project, chapters, activeChapterId, setActiveChapterId, onCreate, onExport, onExportDocx, onBackup, onRestore, onEditProject, onDeleteProject, canDeleteProject, projectBusy, onOpenWorld, onOpenCharacters, onOpenNotes, onMove, onDelete, chapterBusy, activeActivity }: { project?: WorkspaceResponse['workspace']['project']; chapters: Chapter[]; activeChapterId: number | null; setActiveChapterId: (id: number) => void; onCreate: () => void; onExport: () => void; onExportDocx: () => void; onBackup: () => void; onRestore: () => void; onEditProject: () => void; onDeleteProject: () => void; canDeleteProject: boolean; projectBusy: boolean; onOpenWorld: () => void; onOpenCharacters: () => void; onOpenNotes: () => void; onMove: (chapterID: number, direction: -1 | 1) => void; onDelete: (chapter: Chapter) => void; chapterBusy: boolean; activeActivity: ActivityId }) {
  return <div className="explorer-body">
    <div className="project-summary"><div className="project-cover"><BookOpen size={18} /></div><div className="project-summary-copy"><strong>{project?.title ?? '加载中…'}</strong><span>{project?.genre ?? '小说项目'}</span></div><button className="subtle-icon" onClick={onEditProject} title="编辑项目资料" aria-label="编辑项目资料" disabled={!project || projectBusy}><Pencil size={14} /></button><button className="subtle-icon danger" onClick={onDeleteProject} title={canDeleteProject ? '删除当前项目' : '至少保留一个项目'} aria-label={`删除项目 ${project?.title ?? ''}`} disabled={!project || !canDeleteProject || projectBusy}><Trash2 size={14} /></button></div>
    <div className="tree-toolbar"><span>项目文件</span><div><button className="subtle-icon" onClick={onCreate} title="新建章节" aria-label="新建章节" disabled={chapterBusy}><FilePlus2 size={14} /></button><button className="subtle-icon" onClick={onExport} title="导出 Markdown" aria-label={`导出${project?.title ?? '当前项目'}为 Markdown`} disabled={!project || projectBusy}><ArrowDownToLine size={14} /></button><button className="subtle-icon" onClick={onExportDocx} title="导出 Word 文档（DOCX）" aria-label={`导出${project?.title ?? '当前项目'}为 Word 文档`} disabled={!project || projectBusy}><FileText size={14} /></button><button className="subtle-icon" onClick={onBackup} title="导出完整项目备份" aria-label={`导出${project?.title ?? '当前项目'}的完整备份`} disabled={!project || projectBusy}><Archive size={14} /></button><button className="subtle-icon" onClick={onRestore} title="导入项目备份" aria-label="导入项目备份" disabled={!project || projectBusy}><Upload size={14} /></button></div></div>
    <div className="file-tree">
      <div className="tree-folder open"><ChevronDown size={13} /><FolderOpen size={15} className="folder-icon" /><span>{project?.title ?? '未命名项目'}</span></div>
      <div className="tree-folder open nested"><ChevronDown size={13} /><FolderOpen size={15} className="folder-icon" /><span>章节</span><span className="tree-count">{chapters.length}</span></div>
      {chapters.map((chapter, index) => <div key={chapter.id} className={`tree-file nested-2 chapter-tree-row ${activeChapterId === chapter.id ? 'active' : ''}`}>
        <button className="chapter-tree-select" onClick={() => setActiveChapterId(chapter.id)} disabled={chapterBusy}>
          <FileText size={14} className="file-icon" /><span className="tree-chapter-number">{String(chapter.number).padStart(2, '0')}</span><span className="tree-file-name">{chapter.title}</span><span className={`tree-status ${statusClass(chapter.status)}`} />
        </button>
        <div className="chapter-tree-actions">
          <button className="chapter-action-button" onClick={() => onMove(chapter.id, -1)} title="上移章节" aria-label={`上移第 ${chapter.number} 章`} disabled={chapterBusy || index === 0}><ChevronUp size={12} /></button>
          <button className="chapter-action-button" onClick={() => onMove(chapter.id, 1)} title="下移章节" aria-label={`下移第 ${chapter.number} 章`} disabled={chapterBusy || index === chapters.length - 1}><ChevronDown size={12} /></button>
          <button className="chapter-action-button danger" onClick={() => onDelete(chapter)} title="删除章节" aria-label={`删除章节 ${chapter.title}`} disabled={chapterBusy}><Trash2 size={12} /></button>
        </div>
      </div>)}
      <button className="tree-file nested" onClick={onOpenWorld} disabled={!project || projectBusy} aria-label="查看世界观设定"><Archive size={14} className="file-icon muted-icon" /><span className="tree-file-name muted-text">世界观设定</span></button>
      <button className="tree-file nested" onClick={onOpenCharacters} disabled={!project || projectBusy}><Users size={14} className="file-icon muted-icon" /><span className="tree-file-name muted-text">角色档案</span></button>
      <button className="tree-file nested" onClick={onOpenNotes} disabled={!project || projectBusy}><Lightbulb size={14} className="file-icon muted-icon" /><span className="tree-file-name muted-text">灵感便笺</span></button>
    </div>
    {activeActivity === 'chapters' && <div className="sidebar-helper"><div className="helper-icon"><ListTodo size={15} /></div><div><strong>章节视图</strong><p>使用章节右侧的箭头调整顺序。</p></div></div>}
  </div>
}

export function CharactersView({ characters, busy, onCreate, onEdit, onDelete }: { characters: Character[]; busy: boolean; onCreate: () => void; onEdit: (character: Character) => void; onDelete: (character: Character) => void }) {
  return <div className="collection-view"><div className="collection-intro"><div className="collection-icon"><Users size={16} /></div><div><strong>角色档案</strong><p>让每个角色在长篇里保持自己的声音。</p></div></div>{characters.map((character) => <div className="character-card" key={character.id}><div className="character-avatar" style={{ background: `${character.color}22`, color: character.color }}>{character.name.slice(0, 1)}</div><div className="character-info"><strong>{character.name}</strong><span>{character.role}</span><p>{character.description}</p><div className="character-traits">{character.traits.map((trait) => <i key={trait}>{trait}</i>)}</div></div><div className="collection-actions"><button className="subtle-icon" onClick={() => onEdit(character)} aria-label={`编辑角色 ${character.name}`} title="编辑" disabled={busy}><Pencil size={13} /></button><button className="subtle-icon danger" onClick={() => onDelete(character)} aria-label={`删除角色 ${character.name}`} title="删除" disabled={busy}><Trash2 size={13} /></button></div></div>)}{characters.length === 0 && <div className="collection-empty">还没有角色档案，添加角色后 Agent 会把资料带入对话。</div>}<button className="dashed-add" onClick={onCreate} disabled={busy}><Plus size={14} /> 新建角色</button></div>
}

export function NotesView({ notes, busy, onCreate, onEdit, onDelete }: { notes: Note[]; busy: boolean; onCreate: () => void; onEdit: (note: Note) => void; onDelete: (note: Note) => void }) {
  return <div className="collection-view"><div className="collection-intro"><div className="collection-icon warm"><Lightbulb size={16} /></div><div><strong>灵感便笺</strong><p>把还没成熟的想法先放在这里。</p></div></div>{notes.map((note) => <div className="note-card" key={note.id}><div className="note-card-heading"><Lightbulb size={13} /><strong>{note.title}</strong><span>{note.kind}</span><div className="collection-actions"><button className="subtle-icon" onClick={() => onEdit(note)} aria-label={`编辑便笺 ${note.title}`} title="编辑" disabled={busy}><Pencil size={13} /></button><button className="subtle-icon danger" onClick={() => onDelete(note)} aria-label={`删除便笺 ${note.title}`} title="删除" disabled={busy}><Trash2 size={13} /></button></div></div><p>{note.body}</p></div>)}{notes.length === 0 && <div className="collection-empty">还没有便笺，记录下线索、设定或之后要处理的想法。</div>}<button className="dashed-add" onClick={onCreate} disabled={busy}><Plus size={14} /> 新建便笺</button></div>
}

export { TimelineView }
export { WorldAssetsView }

export function SettingsView({ settings, setSettings, onSave, saved, saving, locked, dirty, error, editorPreferences, setEditorPreferences }: { settings: SettingsDraft | null; setSettings: (settings: SettingsDraft) => void; onSave: () => void; saved: boolean; saving: boolean; locked: boolean; dirty: boolean; error: string; editorPreferences: EditorPreferences; setEditorPreferences: (preferences: EditorPreferences) => void }) {
  if (!settings) return <div className="loading-side">加载设置…</div>
  const apiKeyPlaceholder = settings.api_key_configured
    ? '留空以保留已保存的密钥'
    : settings.provider === 'compatible'
      ? '按接口要求填写；本机免鉴权接口可留空'
      : settings.provider === 'openai'
        ? '此服务商需要 API Key，仅保存在本机'
        : '仅保存在本机'
  return (
    <div className="settings-view">
      {error && <p className="dialog-error settings-error" role="alert">{error}</p>}
      <div className="settings-intro"><div className="collection-icon"><Settings2 size={16} /></div><div><strong>模型设置</strong><p>单用户配置保存在本地工作区。</p></div></div>
      <label>服务商<select disabled={saving || locked} value={settings.provider} onChange={(event) => setSettings({ ...settings, provider: event.target.value })}><option value="local">本地创作引擎</option><option value="openai">OpenAI 兼容接口</option><option value="compatible">其他兼容接口</option></select></label>
      <label>模型名称<input disabled={saving || locked} value={settings.model} onChange={(event) => setSettings({ ...settings, model: event.target.value })} placeholder="local-creative" /></label>
      <label>接口地址<input disabled={saving || locked} value={settings.base_url} onChange={(event) => setSettings({ ...settings, base_url: event.target.value })} placeholder="https://api.example.com/v1" /></label><p className="settings-note"><CircleHelp size={13} /> HTTP 仅用于本机回环地址；其他模型服务请使用 HTTPS。</p>
      {settings.base_url_credentials_configured && <button className="settings-key-clear" type="button" disabled={saving || locked} onClick={() => setSettings({ ...settings, clear_base_url_credentials: !settings.clear_base_url_credentials })}>{settings.clear_base_url_credentials ? '撤销清除地址凭据' : '清除接口地址中隐藏的凭据'}</button>}
      {settings.clear_base_url_credentials && <p className="settings-url-warning">保存后将从本地配置中移除 URL 凭据。</p>}
      <label>API Key<input disabled={saving || locked} type="password" autoComplete="new-password" value={settings.api_key ?? ''} onChange={(event) => setSettings({ ...settings, api_key: event.target.value, clear_api_key: false })} placeholder={apiKeyPlaceholder} /></label>
      {settings.api_key_configured && <button className="settings-key-clear" type="button" disabled={saving || locked} onClick={() => setSettings({ ...settings, api_key: '', clear_api_key: !settings.clear_api_key })}>{settings.clear_api_key ? '撤销清除密钥' : '清除已保存的密钥'}</button>}
      {settings.clear_api_key && <p className="settings-key-warning">保存后将删除本地保存的 API Key。</p>}
      <label>创造性 <div className="range-row"><input disabled={saving || locked} type="range" min="0" max="2" step="0.1" value={settings.temperature} onChange={(event) => setSettings({ ...settings, temperature: Number(event.target.value) })} /><span>{settings.temperature.toFixed(1)}</span></div></label>
      <button className="primary-button settings-save" onClick={onSave} disabled={saving || locked || !dirty}>{saving ? '正在保存…' : saved ? <><Check size={14} /> 已保存</> : <><Save size={14} /> 保存设置</>}</button>
      <div className="settings-note"><CircleHelp size={13} /> 密钥只发送到本机服务，不会在设置读取或工作区响应中返回。</div>
      <div className="settings-divider" />
      <div className="settings-subheading">编辑器外观</div>
      <label>正文大小<select value={editorPreferences.fontSize} onChange={(event) => setEditorPreferences({ ...editorPreferences, fontSize: Number(event.target.value) })}><option value={13}>小（13px）</option><option value={14}>标准（14px）</option><option value={15}>舒适（15px）</option><option value={16}>大（16px）</option><option value={18}>特大（18px）</option><option value={20}>超大（20px）</option></select></label>
      <label>正文样式<select value={editorPreferences.fontFamily} onChange={(event) => setEditorPreferences({ ...editorPreferences, fontFamily: event.target.value as EditorPreferences['fontFamily'] })}><option value="serif">衬线字体，适合长篇阅读</option><option value="sans">无衬线字体，界面感更强</option><option value="mono">等宽字体，适合校对</option></select></label>
      <label>界面主题<select value={editorPreferences.theme} onChange={(event) => setEditorPreferences({ ...editorPreferences, theme: event.target.value as EditorPreferences['theme'] })}><option value="dark">深色，适合夜间写作</option><option value="light">浅色，适合明亮环境</option></select></label>
      <label className="settings-checkbox"><input type="checkbox" checked={editorPreferences.focusMode} onChange={(event) => setEditorPreferences({ ...editorPreferences, focusMode: event.target.checked })} /><span>专注写作模式</span><small>隐藏侧栏和 Agent，保留编辑器</small></label>
      <div className="settings-note"><CircleHelp size={13} /> 编辑器偏好保存在当前浏览器，重启后仍会保留。</div>
    </div>
  )
}
