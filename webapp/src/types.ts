// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

export type SavedLinkSummary = {
    id: string;
    teamId: string;
    channelId?: string;
    folderId?: string;
    position: number;
    url: string;
    name: string;
    tags: string[];
    creatorId: string;
    sourcePostId?: string;
    createdAt: number;
    updatedAt: number;
};

export type SavedLink = SavedLinkSummary & {
    version: number;
};

export type Folder = {
    id: string;
    teamId: string;
    channelId?: string;
    name: string;
    creatorId: string;
    createdAt: number;
    updatedAt: number;
    version: number;
};

export type FolderSummary = Omit<Folder, 'version'> & {
    savedLinkCount: number;
};

export type FolderListResult = {
    items: FolderSummary[];
    total: number;
};

export type ListResult<T> = {
    items: T[];
    total: number;
    page: number;
    perPage: number;
    hasMore: boolean;
};

export type SavedLinkScope = 'team' | 'channel' | 'all';

export type SavedLinkQuery = {
    scope: SavedLinkScope;
    teamId?: string;
    channelId?: string;
    folderId?: string;
    search?: string;
    tag?: string;
    page?: number;
    perPage?: number;
};

export type CreateSavedLinkInput = {
    teamId?: string;
    channelId?: string;
    folderId?: string;
    url: string;
    name?: string;
    tags: string[];
    sourcePostId?: string;
};

export type UpdateSavedLinkInput = {
    teamId?: string;
    channelId?: string;
    folderId?: string;
    url?: string;
    name?: string;
    tags?: string[];
    sourcePostId?: string;
};

export type CreateFolderInput = {
    teamId?: string;
    channelId?: string;
    name: string;
};

export type UpdateFolderInput = {
    name?: string;
    teamId?: string;
    channelId?: string;
};

export type ComposerRequest = {
    kind: 'saved-link';
    postId?: string;
    channelId?: string;
    name?: string;
    folderId?: string;
    urls?: string[];
};
