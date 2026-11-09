// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import manifest from 'manifest';

import type {
    Bookmark,
    BookmarkQuery,
    BookmarkSummary,
    CreateBookmarkInput,
    ListResult,
    UpdateBookmarkInput,
} from './types';

let basePath = '';

// Keep only the path of the SiteURL so requests stay same-origin even when the browser reached
// the server through a different hostname.
export function setSiteURL(siteURL: string | undefined) {
    if (!siteURL) {
        basePath = '';
        return;
    }
    try {
        basePath = new URL(siteURL).pathname.replace(/\/$/, '');
    } catch {
        basePath = '';
    }
}

export function getSitePath() {
    return basePath;
}

export class ApiError extends Error {
    status: number;

    constructor(message: string, status: number) {
        super(message);
        this.name = 'ApiError';
        this.status = status;
    }
}

type QueryValue = string | number | boolean | undefined | null;
type RequestOptions = {method?: string; body?: string};

function withQuery(path: string, params: Record<string, QueryValue>) {
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(params)) {
        if (value === undefined || value === null || value === '') {
            continue;
        }
        search.set(key, String(value));
    }
    const query = search.toString();
    return query ? `${path}?${query}` : path;
}

function csrfToken(): string {
    const match = document.cookie.match(/(?:^|;\s*)MMCSRF=([^;]+)/);
    if (!match) {
        return '';
    }
    try {
        return decodeURIComponent(match[1]);
    } catch {
        return match[1];
    }
}

function requestHeaders(method: string, body?: string): Record<string, string> {
    const headers: Record<string, string> = {'X-Requested-With': 'XMLHttpRequest'};
    if (method.toLowerCase() !== 'get') {
        const token = csrfToken();
        if (token) {
            headers['X-CSRF-Token'] = token;
        }
    }
    if (body) {
        headers['Content-Type'] = 'application/json';
    }
    return headers;
}

async function doFetch<T>(path: string, options: RequestOptions = {}, signal?: AbortSignal): Promise<T> {
    // The session cookie authenticates the request. Non-GET requests also carry the MMCSRF token.
    const method = options.method ?? 'get';
    const response = await fetch(
        `${basePath}/plugins/${manifest.id}/api/v1${path}`,
        {
            method,
            body: options.body,
            headers: requestHeaders(method, options.body),
            credentials: 'include',
            signal,
        },
    );

    if (response.status === 204) {
        return undefined as T;
    }

    const text = await response.text();
    let body: unknown = null;
    if (text) {
        try {
            body = JSON.parse(text);
        } catch {
            body = null;
        }
    }
    if (!response.ok) {
        const errorBody = body as {error?: string; message?: string} | null;
        const message = errorBody?.error ?? errorBody?.message ?? (text.trim() || `Request failed with status ${response.status}`);
        throw new ApiError(message, response.status);
    }
    return body as T;
}

export async function fetchBookmarks(query: BookmarkQuery, signal?: AbortSignal): Promise<ListResult<BookmarkSummary>> {
    return doFetch<ListResult<BookmarkSummary>>(
        withQuery('/bookmarks', {
            scope: query.scope,
            team_id: query.teamId,
            channel_id: query.channelId,
            q: query.search,
            tag: query.tag,
            page: query.page,
            per_page: query.perPage,
        }),
        {},
        signal,
    );
}

export async function getBookmark(id: string, signal?: AbortSignal): Promise<Bookmark> {
    return doFetch<Bookmark>(`/bookmarks/${encodeURIComponent(id)}`, {}, signal);
}

export async function createBookmark(input: CreateBookmarkInput): Promise<Bookmark> {
    return doFetch<Bookmark>('/bookmarks', {method: 'post', body: JSON.stringify(input)});
}

export async function updateBookmark(id: string, patch: UpdateBookmarkInput): Promise<Bookmark> {
    return doFetch<Bookmark>(`/bookmarks/${encodeURIComponent(id)}`, {method: 'post', body: JSON.stringify(patch)});
}

export async function deleteBookmark(id: string): Promise<void> {
    await doFetch<void>(`/bookmarks/${encodeURIComponent(id)}`, {method: 'delete'});
}
