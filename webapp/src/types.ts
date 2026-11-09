// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

export type BookmarkSummary = {
    id: string;
    teamId: string;
    channelId?: string;
    url: string;
    title: string;
    tags: string[];
    creatorId: string;
    sourcePostId?: string;
    createdAt: number;
    updatedAt: number;
};

export type Bookmark = BookmarkSummary & {
    description?: string;
    version: number;
};

export type ListResult<T> = {
    items: T[];
    total: number;
    page: number;
    perPage: number;
    hasMore: boolean;
};

export type BookmarkScope = 'team' | 'channel' | 'all';

export type BookmarkQuery = {
    scope: BookmarkScope;
    teamId?: string;
    channelId?: string;
    search?: string;
    tag?: string;
    page?: number;
    perPage?: number;
};

export type CreateBookmarkInput = {
    teamId?: string;
    channelId?: string;
    url: string;
    title: string;
    description?: string;
    tags: string[];
    sourcePostId?: string;
};

export type UpdateBookmarkInput = {
    teamId?: string;
    channelId?: string;
    url?: string;
    title?: string;
    description?: string;
    tags?: string[];
    sourcePostId?: string;
};

export type ComposerRequest = {
    kind: 'bookmark';
    postId?: string;
    channelId?: string;
    title?: string;
    description?: string;
    urls?: string[];
};
