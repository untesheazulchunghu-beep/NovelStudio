import ky from 'ky';

const api = ky.create({
  prefixUrl: '/api',
  headers: {
    'Content-Type': 'application/json',
  },
  hooks: {
    afterResponse: [
      async (_request, _options, response) => {
        const body = await response.json();
        if (!body.success) {
          throw new Error(body.error || 'API request failed');
        }
        return new Response(JSON.stringify(body.data), {
          status: response.status,
          headers: response.headers,
        });
      },
    ],
  },
});

export interface Novel {
  id: number;
  title: string;
  author: string;
  description?: string;
  created_at?: string;
  updated_at?: string;
}

export interface Chapter {
  id: number;
  novel_id: number;
  title: string;
  content: string;
  chapter_number: number;
  word_count: number;
  created_at?: string;
  updated_at?: string;
}

export interface Character {
  id: number;
  novel_id: number;
  name: string;
  description?: string;
  role?: string;
  created_at?: string;
  updated_at?: string;
}

// Novel APIs
export const novelApi = {
  getAll: () => api.get('novels').json<{ novels: Novel[] }>(),

  getOne: (id: number) => api.get(`novels/${id}`).json<Novel>(),

  create: (data: Pick<Novel, 'title' | 'author' | 'description'>) =>
    api.post('novels', { json: data }).json<Novel>(),

  update: (id: number, data: Partial<Novel>) =>
    api.put(`novels/${id}`, { json: data }).json<Novel>(),

  delete: (id: number) =>
    api.delete(`novels/${id}`).json<{ deleted: boolean; id: number }>(),

  getStats: (id: number) =>
    api.get(`novels/${id}/stats`).json<{
      novel_id: number;
      total_chapters: number;
      total_words: number;
    }>(),
};

// Chapter APIs
export const chapterApi = {
  getByNovel: (novelId: number) =>
    api.get(`novels/${novelId}/chapters`).json<{ chapters: Chapter[] }>(),

  create: (novelId: number, data: Pick<Chapter, 'title' | 'content' | 'chapter_number'>) =>
    api.post(`novels/${novelId}/chapters`, { json: data }).json<Chapter>(),

  update: (id: number, data: Partial<Chapter>) =>
    api.put(`chapters/${id}`, { json: data }).json<Chapter>(),

  delete: (id: number) =>
    api.delete(`chapters/${id}`).json<{ deleted: boolean; id: number }>(),
};

// Character APIs
export const characterApi = {
  getByNovel: (novelId: number) =>
    api.get(`novels/${novelId}/characters`).json<{ characters: Character[] }>(),

  create: (novelId: number, data: Pick<Character, 'name' | 'description' | 'role'>) =>
    api.post(`novels/${novelId}/characters`, { json: data }).json<Character>(),
};

// AI APIs
export const aiApi = {
  complete: (prompt: string, system?: string, model?: string) =>
    api.post('ai/complete', {
      json: { prompt, system, model },
    }).json<{ text: string; model: string }>(),

  continue: (text: string, model?: string) =>
    api.post('ai/continue', {
      json: { text, model },
    }).json<{ continuation: string }>(),
};
