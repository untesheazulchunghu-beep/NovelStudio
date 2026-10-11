import { CalendarClock, Check, Circle, Clock3, Pencil, Plus, Trash2 } from 'lucide-react'
import type { Chapter, TimelineEvent } from '../types'

const text = {
  title: '\u6545\u4e8b\u65f6\u95f4\u7ebf',
  intro: '\u6309\u6545\u4e8b\u987a\u5e8f\u8bb0\u5f55\u5173\u952e\u4e8b\u4ef6\uff0c\u7ae0\u8282\u662f\u627f\u8f7d\u8fd9\u4e9b\u53d8\u5316\u7684\u5165\u53e3\u3002',
  empty: '\u8fd8\u6ca1\u6709\u65f6\u95f4\u7ebf\u4e8b\u4ef6\u3002\u5148\u8bb0\u5f55\u4e00\u4e2a\u5fc5\u987b\u53d1\u751f\u7684\u8f6c\u6298\u6216\u7ed3\u679c\u3002',
  edit: '\u7f16\u8f91\u65f6\u95f4\u7ebf\u4e8b\u4ef6',
  delete: '\u5220\u9664\u65f6\u95f4\u7ebf\u4e8b\u4ef6',
  add: '\u65b0\u5efa\u65f6\u95f4\u7ebf\u4e8b\u4ef6',
}

export function TimelineView({ events, chapters, busy, onCreate, onEdit, onDelete, onOpenChapter }: { events: TimelineEvent[]; chapters: Chapter[]; busy: boolean; onCreate: () => void; onEdit: (event: TimelineEvent) => void; onDelete: (event: TimelineEvent) => void; onOpenChapter: (chapterID: number) => void }) {
  const chapterTitle = new Map(chapters.map((chapter) => [chapter.id, `\u7b2c${chapter.number} \u7ae0 \u00b7 ${chapter.title}`]))
  return <div className="timeline-view">
    <div className="collection-intro"><div className="collection-icon"><CalendarClock size={16} /></div><div><strong>{text.title}</strong><p>{text.intro}</p></div></div>
    {events.length === 0 && <div className="collection-empty">{text.empty}</div>}
    <div className="timeline-list">
      {events.map((event) => <article className={`timeline-card ${event.status}`} key={event.id}>
        <div className="timeline-marker" aria-hidden="true">{event.status === 'done' ? <Check size={11} /> : event.status === 'active' ? <Clock3 size={11} /> : <Circle size={9} />}</div>
        <div className="timeline-card-body">
          <div className="timeline-card-heading"><div><strong>{event.title}</strong>{event.time_label && <span>{event.time_label}</span>}</div><div className="collection-actions"><button className="subtle-icon" onClick={() => onEdit(event)} aria-label={`${text.edit}: ${event.title}`} title={text.edit} disabled={busy}><Pencil size={13} /></button><button className="subtle-icon danger" onClick={() => onDelete(event)} aria-label={`${text.delete}: ${event.title}`} title={text.delete} disabled={busy}><Trash2 size={13} /></button></div></div>
          {event.description && <p>{event.description}</p>}
          {typeof event.chapter_id === 'number' && event.chapter_id > 0 && chapterTitle.has(event.chapter_id) && <button type="button" className="timeline-chapter" onClick={() => onOpenChapter(event.chapter_id!)} disabled={busy}>{chapterTitle.get(event.chapter_id)}</button>}
        </div>
      </article>)}
    </div>
    <button className="dashed-add" onClick={onCreate} disabled={busy}><Plus size={14} /> {text.add}</button>
  </div>
}
