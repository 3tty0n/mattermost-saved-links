// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import manifest from 'manifest';

import type {
    SavedLink,
    SavedLinkQuery,
    SavedLinkSummary,
    CreateSavedLinkInput,
    CreateFolderInput,
    Folder,
    FolderListResult,
    ListResult,
    UpdateSavedLinkInput,
    UpdateFolderInput,
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

export async function fetchSavedLinks(query: SavedLinkQuery, signal?: AbortSignal): Promise<ListResult<SavedLinkSummary>> {
    return doFetch<ListResult<SavedLinkSummary>>(
        withQuery('/saved-links', {
            scope: query.scope,
            team_id: query.teamId,
            channel_id: query.channelId,
            folder_id: query.folderId,
            q: query.search,
            tag: query.tag,
            page: query.page,
            per_page: query.perPage,
        }),
        {},
        signal,
    );
}

export async function fetchFolders(teamId: string | undefined, channelId: string | undefined, signal?: AbortSignal): Promise<FolderListResult> {
    return doFetch<FolderListResult>(
        withQuery('/folders', {
            team_id: channelId ? undefined : teamId,
            channel_id: channelId,
        }),
        {},
        signal,
    );
}

export async function getSavedLink(id: string, signal?: AbortSignal): Promise<SavedLink> {
    return doFetch<SavedLink>(`/saved-links/${encodeURIComponent(id)}`, {}, signal);
}

export async function createSavedLink(input: CreateSavedLinkInput): Promise<SavedLink> {
    return doFetch<SavedLink>('/saved-links', {method: 'post', body: JSON.stringify(input)});
}

export async function updateSavedLink(id: string, patch: UpdateSavedLinkInput): Promise<SavedLink> {
    return doFetch<SavedLink>(`/saved-links/${encodeURIComponent(id)}`, {method: 'post', body: JSON.stringify(patch)});
}

export async function deleteSavedLink(id: string): Promise<void> {
    await doFetch<void>(`/saved-links/${encodeURIComponent(id)}`, {method: 'delete'});
}

export async function reorderSavedLinks(savedLinkIds: string[]): Promise<void> {
    await doFetch<void>('/saved-links/reorder', {method: 'post', body: JSON.stringify({savedLinkIds})});
}

export async function createFolder(input: CreateFolderInput): Promise<Folder> {
    return doFetch<Folder>('/folders', {method: 'post', body: JSON.stringify(input)});
}

export async function updateFolder(id: string, patch: UpdateFolderInput): Promise<Folder> {
    return doFetch<Folder>(`/folders/${encodeURIComponent(id)}`, {method: 'post', body: JSON.stringify(patch)});
}

export async function deleteFolder(id: string): Promise<void> {
    await doFetch<void>(`/folders/${encodeURIComponent(id)}`, {method: 'delete'});
}
