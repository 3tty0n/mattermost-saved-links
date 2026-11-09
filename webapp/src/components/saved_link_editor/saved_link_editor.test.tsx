// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {createSavedLink, fetchFolders, updateSavedLink} from 'client';

import type {SavedLink, FolderSummary} from 'types';

import SavedLinkEditor from './saved_link_editor';

import {renderWithStore, ids} from '../../../tests/test_utils';

jest.mock('client', () => ({
    ApiError: class ApiError extends Error {},
    createSavedLink: jest.fn(),
    createFolder: jest.fn(),
    deleteSavedLink: jest.fn(),
    deleteFolder: jest.fn(),
    fetchSavedLinks: jest.fn(),
    fetchFolders: jest.fn(),
    getSavedLink: jest.fn(),
    getSitePath: jest.fn(() => ''),
    reorderSavedLinks: jest.fn(),
    setSiteURL: jest.fn(),
    updateSavedLink: jest.fn(),
    updateFolder: jest.fn(),
}));

const createSavedLinkMock = createSavedLink as jest.MockedFunction<typeof createSavedLink>;
const updateSavedLinkMock = updateSavedLink as jest.MockedFunction<typeof updateSavedLink>;
const fetchFoldersMock = fetchFolders as jest.MockedFunction<typeof fetchFolders>;

const folder: FolderSummary = {
    id: 'folder-1',
    teamId: ids.team,
    channelId: ids.channel,
    name: 'Work',
    creatorId: ids.user,
    savedLinkCount: 0,
    createdAt: 1,
    updatedAt: 1,
};

const existingSavedLink: SavedLink = {
    id: 'link-1',
    teamId: ids.team,
    channelId: ids.channel,
    folderId: folder.id,
    position: 1000,
    url: 'https://example.com/one',
    name: 'Docs',
    tags: ['work', 'docs'],
    creatorId: ids.user,
    createdAt: 1,
    updatedAt: 1,
    version: 1,
};

describe('SavedLinkEditor', () => {
    beforeEach(() => {
        createSavedLinkMock.mockResolvedValue(existingSavedLink);
        updateSavedLinkMock.mockResolvedValue(existingSavedLink);
        fetchFoldersMock.mockResolvedValue({items: [folder], total: 1});
    });

    it('creates a saved link from a post with the selected URL and folder', async () => {
        const user = userEvent.setup();
        const onSaved = jest.fn();
        renderWithStore(
            <SavedLinkEditor
                defaults={{
                    teamId: ids.team,
                    channelId: ids.channel,
                    urls: ['https://example.com/one', 'https://example.com/two'],
                    name: 'Docs',
                    sourcePostId: 'post-id',
                }}
                onClose={jest.fn()}
                onSaved={onSaved}
            />,
        );

        await user.click(screen.getByRole('combobox', {name: 'URL from the post'}));
        await user.click(await screen.findByRole('option', {name: 'https://example.com/two'}));
        await user.click(await screen.findByRole('combobox', {name: 'Folder'}));
        await user.click(await screen.findByRole('option', {name: 'Work'}));
        await user.type(screen.getByLabelText('Tags (comma separated)'), 'work, docs');
        await user.click(screen.getByRole('button', {name: 'Save link'}));

        await waitFor(() => expect(createSavedLinkMock).toHaveBeenCalledWith(expect.objectContaining({
            teamId: ids.team,
            channelId: ids.channel,
            folderId: folder.id,
            url: 'https://example.com/two',
            name: 'Docs',
            tags: ['work', 'docs'],
            sourcePostId: 'post-id',
        })));
        expect(onSaved).toHaveBeenCalledWith(existingSavedLink);
    });

    it('saves a saved link with only a URL', async () => {
        const user = userEvent.setup();
        renderWithStore(
            <SavedLinkEditor
                defaults={{teamId: ids.team, channelId: ids.channel}}
                onClose={jest.fn()}
                onSaved={jest.fn()}
            />,
        );

        await user.click(screen.getByRole('button', {name: 'Save link'}));
        expect(await screen.findByText('Enter a URL.')).toBeVisible();

        await user.type(screen.getByLabelText('URL'), 'https://example.com/getting-started');
        await user.click(screen.getByRole('button', {name: 'Save link'}));

        await waitFor(() => expect(createSavedLinkMock).toHaveBeenCalled());
        const input = createSavedLinkMock.mock.calls[0][0];
        expect(input.url).toBe('https://example.com/getting-started');
        expect(input.name).toBeUndefined();
        expect(input.folderId).toBeUndefined();
    });

    it('requires a URL and keeps the dialog open on error', async () => {
        const user = userEvent.setup();
        createSavedLinkMock.mockRejectedValueOnce(new Error('Only http and https URLs are supported'));
        renderWithStore(
            <SavedLinkEditor
                defaults={{teamId: ids.team, channelId: ids.channel}}
                onClose={jest.fn()}
                onSaved={jest.fn()}
            />,
        );

        await user.type(screen.getByLabelText('URL'), 'https://example.com/one');
        await user.click(screen.getByRole('button', {name: 'Save link'}));
        expect(await screen.findByText('Only http and https URLs are supported')).toBeVisible();
    });

    it('updates an existing saved link and its folder', async () => {
        const user = userEvent.setup();
        const onSaved = jest.fn();
        renderWithStore(
            <SavedLinkEditor
                savedLink={existingSavedLink}
                onClose={jest.fn()}
                onSaved={onSaved}
            />,
        );

        const name = screen.getByLabelText('Name');
        await user.clear(name);
        await user.type(name, 'Renamed');
        await user.click(screen.getByRole('button', {name: 'Save'}));

        await waitFor(() => expect(updateSavedLinkMock).toHaveBeenCalledWith('link-1', expect.objectContaining({
            name: 'Renamed',
            url: 'https://example.com/one',
            folderId: folder.id,
            tags: ['work', 'docs'],
        })));
        expect(onSaved).toHaveBeenCalledWith(existingSavedLink);
    });

    it('moves a saved link out of its folder', async () => {
        const user = userEvent.setup();
        renderWithStore(
            <SavedLinkEditor
                savedLink={existingSavedLink}
                onClose={jest.fn()}
                onSaved={jest.fn()}
            />,
        );

        await user.click(await screen.findByRole('combobox', {name: 'Folder'}));
        await user.click(await screen.findByRole('option', {name: 'No folder'}));
        await user.click(screen.getByRole('button', {name: 'Save'}));

        await waitFor(() => expect(updateSavedLinkMock).toHaveBeenCalledWith('link-1', expect.objectContaining({
            folderId: '',
        })));
    });
});
