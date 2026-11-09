// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {screen} from '@testing-library/react';
import {fetchBookmarks} from 'client';

import type {BookmarkSummary, ListResult} from 'types';

import BookmarksSidebar from './bookmarks_sidebar';

import {renderWithStore, ids} from '../../../tests/test_utils';

jest.mock('client', () => ({
    ApiError: class ApiError extends Error {},
    createBookmark: jest.fn(),
    createTask: jest.fn(),
    deleteBookmark: jest.fn(),
    deleteTask: jest.fn(),
    fetchAssignees: jest.fn(),
    fetchBookmarks: jest.fn(),
    fetchTasks: jest.fn(),
    getBookmark: jest.fn(),
    getProfiles: jest.fn(),
    getProfilePictureUrl: jest.fn(),
    getSitePath: jest.fn(() => ''),
    getTask: jest.fn(),
    setSiteURL: jest.fn(),
    updateBookmark: jest.fn(),
    updateTask: jest.fn(),
}));

const bookmark: BookmarkSummary = {
    id: 'bookmark-1',
    teamId: ids.team,
    channelId: ids.channel,
    url: 'https://example.com/runbook',
    title: 'Runbook',
    tags: ['ops'],
    creatorId: ids.user,
    createdAt: 1,
    updatedAt: 1,
};

const page: ListResult<BookmarkSummary> = {items: [bookmark], total: 1, page: 0, perPage: 20, hasMore: false};

describe('BookmarksSidebar', () => {
    it('renders bookmarks in their own panel without task views', async () => {
        (fetchBookmarks as jest.Mock).mockResolvedValue(page);
        renderWithStore(<BookmarksSidebar/>);

        expect(await screen.findByRole('link', {name: 'Runbook'})).toBeVisible();
        expect(screen.queryByRole('button', {name: 'New task'})).not.toBeInTheDocument();
    });
});
