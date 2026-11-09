// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {createBookmark, updateBookmark} from 'client';

import type {Bookmark} from 'types';

import BookmarkEditor from './bookmark_editor';

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

const createBookmarkMock = createBookmark as jest.MockedFunction<typeof createBookmark>;
const updateBookmarkMock = updateBookmark as jest.MockedFunction<typeof updateBookmark>;

const savedBookmark: Bookmark = {
    id: 'bookmark-1',
    teamId: ids.team,
    channelId: ids.channel,
    url: 'https://example.com/one',
    title: 'Docs',
    description: 'Useful',
    tags: ['work', 'docs'],
    creatorId: ids.user,
    createdAt: 1,
    updatedAt: 1,
    version: 1,
};

describe('BookmarkEditor', () => {
    beforeEach(() => {
        createBookmarkMock.mockResolvedValue(savedBookmark);
        updateBookmarkMock.mockResolvedValue(savedBookmark);
    });

    it('creates a bookmark from a post with the selected URL', async () => {
        const user = userEvent.setup();
        const onSaved = jest.fn();
        renderWithStore(
            <BookmarkEditor
                defaults={{
                    teamId: ids.team,
                    channelId: ids.channel,
                    urls: ['https://example.com/one', 'https://example.com/two'],
                    title: 'Docs',
                    sourcePostId: 'post-id',
                }}
                onClose={jest.fn()}
                onSaved={onSaved}
            />,
        );

        await user.click(screen.getByRole('combobox', {name: 'URL from the post'}));
        await user.click(await screen.findByRole('option', {name: 'https://example.com/two'}));
        await user.type(screen.getByLabelText('Tags (comma separated)'), 'work, docs');
        await user.click(screen.getByRole('button', {name: 'Save bookmark'}));

        await waitFor(() => expect(createBookmarkMock).toHaveBeenCalledWith(expect.objectContaining({
            teamId: ids.team,
            channelId: ids.channel,
            url: 'https://example.com/two',
            title: 'Docs',
            tags: ['work', 'docs'],
            sourcePostId: 'post-id',
        })));
        expect(onSaved).toHaveBeenCalledWith(savedBookmark);
    });

    it('requires a URL and keeps the dialog open on error', async () => {
        const user = userEvent.setup();
        createBookmarkMock.mockRejectedValueOnce(new Error('Only http and https URLs are supported'));
        renderWithStore(
            <BookmarkEditor
                defaults={{teamId: ids.team, channelId: ids.channel}}
                onClose={jest.fn()}
                onSaved={jest.fn()}
            />,
        );

        await user.click(screen.getByRole('button', {name: 'Save bookmark'}));
        expect(await screen.findByText('Enter a URL.')).toBeVisible();

        await user.type(screen.getByLabelText('URL'), 'https://example.com/one');
        await user.type(screen.getByLabelText('Title'), 'Docs');
        await user.click(screen.getByRole('button', {name: 'Save bookmark'}));
        expect(await screen.findByText('Only http and https URLs are supported')).toBeVisible();
    });

    it('updates an existing bookmark', async () => {
        const user = userEvent.setup();
        const onSaved = jest.fn();
        renderWithStore(
            <BookmarkEditor
                bookmark={savedBookmark}
                onClose={jest.fn()}
                onSaved={onSaved}
            />,
        );

        const title = screen.getByLabelText('Title');
        await user.clear(title);
        await user.type(title, 'Renamed');
        await user.click(screen.getByRole('button', {name: 'Save'}));

        await waitFor(() => expect(updateBookmarkMock).toHaveBeenCalledWith('bookmark-1', expect.objectContaining({
            title: 'Renamed',
            url: 'https://example.com/one',
            tags: ['work', 'docs'],
        })));
        expect(onSaved).toHaveBeenCalledWith(savedBookmark);
    });
});
