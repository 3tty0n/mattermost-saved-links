// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {deleteBookmark, fetchBookmarks} from 'client';

import type {BookmarkSummary, ListResult} from 'types';

import BookmarkPanel from './bookmark_panel';

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

const fetchBookmarksMock = fetchBookmarks as jest.MockedFunction<typeof fetchBookmarks>;
const deleteBookmarkMock = deleteBookmark as jest.MockedFunction<typeof deleteBookmark>;

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

function page(items: BookmarkSummary[]): ListResult<BookmarkSummary> {
    return {items, total: items.length, page: 0, perPage: 20, hasMore: false};
}

describe('BookmarkPanel', () => {
    beforeEach(() => {
        fetchBookmarksMock.mockResolvedValue(page([bookmark]));
        deleteBookmarkMock.mockResolvedValue();
    });

    it('lists bookmarks with tags and a safe link', async () => {
        renderWithStore(
            <BookmarkPanel
                teamId={ids.team}
                channelId={ids.channel}
                revision={0}
                onCreate={jest.fn()}
                onEdit={jest.fn()}
                onDeleted={jest.fn()}
            />,
        );

        const link = await screen.findByRole('link', {name: 'Runbook'});
        expect(link).toHaveAttribute('href', 'https://example.com/runbook');
        expect(link).toHaveAttribute('target', '_blank');
        expect(screen.getByText('ops')).toBeVisible();
        expect(screen.getByText('Town Square')).toBeVisible();
    });

    it('searches bookmarks', async () => {
        const user = userEvent.setup();
        renderWithStore(
            <BookmarkPanel
                teamId={ids.team}
                channelId={ids.channel}
                revision={0}
                onCreate={jest.fn()}
                onEdit={jest.fn()}
                onDeleted={jest.fn()}
            />,
        );
        await screen.findByRole('link', {name: 'Runbook'});

        await user.type(screen.getByLabelText('Search bookmarks'), 'runbook');
        await waitFor(() => expect(fetchBookmarksMock).toHaveBeenLastCalledWith(
            expect.objectContaining({search: 'runbook', scope: 'channel'}),
            expect.anything(),
        ), {timeout: 2000});
    });

    it('edits and deletes a bookmark', async () => {
        const user = userEvent.setup();
        const onEdit = jest.fn();
        const onDeleted = jest.fn();
        renderWithStore(
            <BookmarkPanel
                teamId={ids.team}
                channelId={ids.channel}
                revision={0}
                onCreate={jest.fn()}
                onEdit={onEdit}
                onDeleted={onDeleted}
            />,
        );
        await screen.findByRole('link', {name: 'Runbook'});

        await user.click(screen.getByRole('button', {name: 'Edit bookmark'}));
        expect(onEdit).toHaveBeenCalledWith('bookmark-1');

        await user.click(screen.getByRole('button', {name: 'Delete bookmark'}));
        await user.click(screen.getByRole('button', {name: 'Delete'}));

        await waitFor(() => expect(deleteBookmarkMock).toHaveBeenCalledWith('bookmark-1'));
        expect(onDeleted).toHaveBeenCalled();
    });

    it('shows the empty state', async () => {
        fetchBookmarksMock.mockResolvedValue(page([]));
        renderWithStore(
            <BookmarkPanel
                teamId={ids.team}
                channelId={ids.channel}
                revision={0}
                onCreate={jest.fn()}
                onEdit={jest.fn()}
                onDeleted={jest.fn()}
            />,
        );

        expect(await screen.findByText('No bookmarks yet')).toBeVisible();
    });

    it('shows a load error', async () => {
        fetchBookmarksMock.mockRejectedValueOnce(new Error('Network down'));
        renderWithStore(
            <BookmarkPanel
                teamId={ids.team}
                channelId={ids.channel}
                revision={0}
                onCreate={jest.fn()}
                onEdit={jest.fn()}
                onDeleted={jest.fn()}
            />,
        );

        expect(await screen.findByText('Couldn’t load bookmarks')).toBeVisible();
        expect(screen.getByText('Network down')).toBeVisible();
    });
});
