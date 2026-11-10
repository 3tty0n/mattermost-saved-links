// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {fireEvent, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {createFolder, deleteSavedLink, deleteFolder, fetchSavedLinks, fetchFolders, reorderSavedLinks, updateFolder, updateSavedLink} from 'client';

import type {SavedLinkSummary, Folder, FolderSummary, ListResult} from 'types';

import SavedLinkPanel from './saved_link_panel';

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

const fetchSavedLinksMock = fetchSavedLinks as jest.MockedFunction<typeof fetchSavedLinks>;
const fetchFoldersMock = fetchFolders as jest.MockedFunction<typeof fetchFolders>;
const deleteSavedLinkMock = deleteSavedLink as jest.MockedFunction<typeof deleteSavedLink>;
const createFolderMock = createFolder as jest.MockedFunction<typeof createFolder>;
const updateFolderMock = updateFolder as jest.MockedFunction<typeof updateFolder>;
const updateSavedLinkMock = updateSavedLink as jest.MockedFunction<typeof updateSavedLink>;
const deleteFolderMock = deleteFolder as jest.MockedFunction<typeof deleteFolder>;
const reorderSavedLinksMock = reorderSavedLinks as jest.MockedFunction<typeof reorderSavedLinks>;

const savedLink: SavedLinkSummary = {
    id: 'link-1',
    teamId: ids.team,
    channelId: ids.channel,
    position: 1000,
    url: 'https://example.com/runbook',
    name: 'Runbook',
    tags: ['ops'],
    creatorId: ids.user,
    createdAt: 1,
    updatedAt: 1,
};

const secondSavedLink: SavedLinkSummary = {
    id: 'link-2',
    teamId: ids.team,
    channelId: ids.channel,
    position: 2000,
    url: 'https://example.com/design',
    name: 'Design',
    tags: ['design'],
    creatorId: ids.user,
    createdAt: 2,
    updatedAt: 2,
};

const folderSummary: FolderSummary = {
    id: 'folder-1',
    teamId: ids.team,
    channelId: ids.channel,
    name: 'Work',
    creatorId: ids.user,
    savedLinkCount: 1,
    createdAt: 1,
    updatedAt: 1,
};

const folder: Folder = {...folderSummary, version: 1};

function page(items: SavedLinkSummary[]): ListResult<SavedLinkSummary> {
    return {items, total: items.length, page: 0, perPage: 100, hasMore: false};
}

function renderPanel(overrides?: {onCreate?: jest.Mock; onEdit?: jest.Mock; onDeleted?: jest.Mock}) {
    return renderWithStore(
        <SavedLinkPanel
            teamId={ids.team}
            channelId={ids.channel}
            revision={0}
            onCreate={overrides?.onCreate ?? jest.fn()}
            onEdit={overrides?.onEdit ?? jest.fn()}
            onDeleted={overrides?.onDeleted ?? jest.fn()}
        />,
    );
}

async function openWorkFolder(user: ReturnType<typeof userEvent.setup>) {
    const folderButton = await screen.findByRole('button', {name: /Open folder Work/});
    await user.click(folderButton);
}

describe('SavedLinkPanel', () => {
    beforeEach(() => {
        fetchSavedLinksMock.mockResolvedValue(page([savedLink]));
        fetchFoldersMock.mockResolvedValue({items: [], total: 0});
        deleteSavedLinkMock.mockResolvedValue();
        createFolderMock.mockResolvedValue(folder);
        updateFolderMock.mockResolvedValue(folder);
        updateSavedLinkMock.mockResolvedValue({...savedLink, version: 1});
        deleteFolderMock.mockResolvedValue();
        reorderSavedLinksMock.mockResolvedValue();
    });

    it('lists ungrouped saved links at the root with tags and a safe link', async () => {
        renderPanel();

        const link = await screen.findByRole('link', {name: 'Runbook'});
        expect(link).toHaveAttribute('href', 'https://example.com/runbook');
        expect(link).toHaveAttribute('target', '_blank');
        expect(screen.getByText('ops')).toBeVisible();
        expect(screen.getByText('Town Square')).toBeVisible();
        expect(fetchSavedLinksMock).toHaveBeenLastCalledWith(
            expect.objectContaining({scope: 'channel', folderId: 'none'}),
            expect.anything(),
        );
    });

    it('shows the folder of a saved link', async () => {
        fetchSavedLinksMock.mockResolvedValue(page([{...savedLink, folderId: folderSummary.id}]));
        fetchFoldersMock.mockResolvedValue({items: [folderSummary], total: 1});
        renderPanel();

        const link = await screen.findByRole('link', {name: 'Runbook'});
        const row = link.closest('li');
        expect(row).not.toBeNull();
        expect(within(row as HTMLElement).getByText('Work')).toBeVisible();
    });

    it('shows folders inline, opens one, and goes back to the root', async () => {
        const user = userEvent.setup();
        fetchSavedLinksMock.mockResolvedValue(page([]));
        fetchFoldersMock.mockResolvedValue({items: [folderSummary], total: 1});
        renderPanel();

        const folderButton = await screen.findByRole('button', {name: /Open folder Work, 1 saved link/});
        expect(folderButton).toBeVisible();

        await user.click(folderButton);
        expect(await screen.findByRole('button', {name: 'Back to all saved links'})).toBeVisible();
        await waitFor(() => expect(fetchSavedLinksMock).toHaveBeenLastCalledWith(
            expect.objectContaining({scope: 'channel', folderId: folderSummary.id}),
            expect.anything(),
        ));
        expect(await screen.findByText('This folder is empty')).toBeVisible();

        await user.click(screen.getByRole('button', {name: 'Back to all saved links'}));
        await waitFor(() => expect(fetchSavedLinksMock).toHaveBeenLastCalledWith(
            expect.objectContaining({scope: 'channel', folderId: 'none'}),
            expect.anything(),
        ));
        expect(await screen.findByRole('button', {name: /Open folder Work/})).toBeVisible();
    });

    it('searches saved links', async () => {
        const user = userEvent.setup();
        renderPanel();
        await screen.findByRole('link', {name: 'Runbook'});

        await user.type(screen.getByLabelText('Search saved links'), 'runbook');
        await waitFor(() => expect(fetchSavedLinksMock).toHaveBeenLastCalledWith(
            expect.objectContaining({search: 'runbook', scope: 'channel'}),
            expect.anything(),
        ), {timeout: 2000});
    });

    it('shows the search empty state', async () => {
        const user = userEvent.setup();
        fetchSavedLinksMock.mockResolvedValueOnce(page([savedLink])).mockResolvedValue(page([]));
        renderPanel();
        await screen.findByRole('link', {name: 'Runbook'});

        await user.type(screen.getByLabelText('Search saved links'), 'nothing');
        expect(await screen.findByText('No saved links match your search')).toBeVisible();
    });

    it('creates a folder in the current scope', async () => {
        const user = userEvent.setup();
        renderPanel();
        await screen.findByRole('link', {name: 'Runbook'});

        await user.click(screen.getByRole('button', {name: 'New folder'}));
        await user.type(screen.getByLabelText('Folder name'), 'Ideas');
        await user.click(within(screen.getByRole('dialog')).getByRole('button', {name: 'Create folder'}));

        await waitFor(() => expect(createFolderMock).toHaveBeenCalledWith(expect.objectContaining({
            channelId: ids.channel,
            name: 'Ideas',
        })));
    });

    it('renames a folder from its folder view', async () => {
        const user = userEvent.setup();
        fetchFoldersMock.mockResolvedValue({items: [folderSummary], total: 1});
        renderPanel();
        await screen.findByRole('link', {name: 'Runbook'});
        await openWorkFolder(user);

        await user.click(screen.getByRole('button', {name: 'Rename folder'}));
        const input = screen.getByLabelText('Folder name');
        await user.clear(input);
        await user.type(input, 'Reference');
        await user.click(within(screen.getByRole('dialog')).getByRole('button', {name: 'Rename folder'}));

        await waitFor(() => expect(updateFolderMock).toHaveBeenCalledWith(folderSummary.id, {name: 'Reference'}));
    });

    it('moves a saved link to another channel', async () => {
        const user = userEvent.setup();
        const onDeleted = jest.fn();
        renderPanel({onDeleted});
        await screen.findByRole('link', {name: 'Runbook'});

        await user.click(screen.getByRole('button', {name: 'Move saved link'}));
        const dialog = within(screen.getByRole('dialog'));
        expect(dialog.getByRole('button', {name: 'Move'})).toBeDisabled();
        await user.click(dialog.getByRole('combobox', {name: 'Scope'}));
        await user.click(await screen.findByRole('option', {name: 'Off-Topic'}));
        await user.click(dialog.getByRole('button', {name: 'Move'}));

        await waitFor(() => expect(updateSavedLinkMock).toHaveBeenCalledWith(savedLink.id, {teamId: ids.team, channelId: ids.otherChannel}));
        expect(onDeleted).toHaveBeenCalled();
    });

    it('lists team-level first and channels alphabetically in the move dialog', async () => {
        const user = userEvent.setup();
        renderPanel();
        await screen.findByRole('link', {name: 'Runbook'});

        await user.click(screen.getByRole('button', {name: 'Move saved link'}));
        await user.click(within(screen.getByRole('dialog')).getByRole('combobox', {name: 'Scope'}));

        expect((await screen.findAllByRole('option')).map((option) => option.textContent)).toEqual(['Team-level (no channel)', 'Off-Topic', 'Town Square']);
    });

    it('moves a folder to team level', async () => {
        const user = userEvent.setup();
        fetchFoldersMock.mockResolvedValue({items: [folderSummary], total: 1});
        renderPanel();
        await screen.findByRole('link', {name: 'Runbook'});
        await openWorkFolder(user);

        await user.click(screen.getByRole('button', {name: 'Move folder'}));
        const dialog = within(screen.getByRole('dialog'));
        await user.click(dialog.getByRole('combobox', {name: 'Scope'}));
        await user.click(await screen.findByRole('option', {name: 'Team-level (no channel)'}));
        await user.click(dialog.getByRole('button', {name: 'Move'}));

        await waitFor(() => expect(updateFolderMock).toHaveBeenCalledWith(folderSummary.id, {teamId: ids.team, channelId: ''}));
    });

    it('moves a folder from the folder list to another channel', async () => {
        const user = userEvent.setup();
        fetchFoldersMock.mockResolvedValue({items: [folderSummary], total: 1});
        renderPanel();

        await user.click(await screen.findByRole('button', {name: 'Move folder Work'}));
        const dialog = within(screen.getByRole('dialog'));
        await user.click(dialog.getByRole('combobox', {name: 'Scope'}));
        await user.click(await screen.findByRole('option', {name: 'Off-Topic'}));
        await user.click(dialog.getByRole('button', {name: 'Move'}));

        await waitFor(() => expect(updateFolderMock).toHaveBeenCalledWith(folderSummary.id, {teamId: ids.team, channelId: ids.otherChannel}));
    });

    it('deletes a folder and refreshes the list', async () => {
        const user = userEvent.setup();
        const onDeleted = jest.fn();
        fetchFoldersMock.mockResolvedValue({items: [folderSummary], total: 1});
        renderPanel({onDeleted});
        await screen.findByRole('link', {name: 'Runbook'});
        await openWorkFolder(user);

        await user.click(screen.getByRole('button', {name: 'Delete folder'}));
        await user.click(within(screen.getByRole('dialog')).getByRole('button', {name: 'Delete folder'}));

        await waitFor(() => expect(deleteFolderMock).toHaveBeenCalledWith(folderSummary.id));
        expect(onDeleted).toHaveBeenCalled();
    });

    it('creates a saved link in the open folder', async () => {
        const user = userEvent.setup();
        const onCreate = jest.fn();
        fetchFoldersMock.mockResolvedValue({items: [folderSummary], total: 1});
        renderPanel({onCreate});
        await screen.findByRole('link', {name: 'Runbook'});
        await openWorkFolder(user);

        await user.click(screen.getByRole('button', {name: 'New saved link'}));

        expect(onCreate).toHaveBeenCalledWith('channel', folderSummary.id);
    });

    it('reorders saved links with the keyboard', async () => {
        const user = userEvent.setup();
        fetchSavedLinksMock.mockResolvedValue(page([savedLink, secondSavedLink]));
        renderPanel();
        await screen.findByRole('link', {name: 'Runbook'});

        const handles = await screen.findAllByRole('button', {name: /Drag to reorder/});
        expect(handles).toHaveLength(2);
        handles[1].focus();
        await user.keyboard('{ArrowUp}');

        await waitFor(() => expect(reorderSavedLinksMock).toHaveBeenCalledWith(['link-2', 'link-1']));
    });

    it('reorders twice in a row with the keyboard', async () => {
        const user = userEvent.setup();
        let current = [savedLink, secondSavedLink];
        fetchSavedLinksMock.mockImplementation(async () => page(current));
        reorderSavedLinksMock.mockImplementation(async (ids) => {
            const byID = new Map(current.map((item) => [item.id, item]));
            current = ids.map((id) => byID.get(id) as SavedLinkSummary);
        });
        renderPanel();
        await screen.findByRole('link', {name: 'Runbook'});

        const handles = await screen.findAllByRole('button', {name: /Drag to reorder/});
        handles[1].focus();
        await user.keyboard('{ArrowUp}');
        await waitFor(() => expect(reorderSavedLinksMock).toHaveBeenCalledTimes(1));
        await waitFor(() => expect(screen.getAllByRole('link').map((link) => link.textContent)).toEqual(['Design', 'Runbook']));
        await user.keyboard('{ArrowDown}');
        await waitFor(() => expect(reorderSavedLinksMock).toHaveBeenCalledTimes(2));
        expect(reorderSavedLinksMock).toHaveBeenLastCalledWith(['link-1', 'link-2']);
    });

    it('reorders twice in a row by dragging', async () => {
        let current = [savedLink, secondSavedLink];
        fetchSavedLinksMock.mockImplementation(async () => page(current));
        reorderSavedLinksMock.mockImplementation(async (ids) => {
            const byID = new Map(current.map((item) => [item.id, item]));
            current = ids.map((id) => byID.get(id) as SavedLinkSummary);
        });
        renderPanel();
        const firstLink = await screen.findByRole('link', {name: 'Runbook'});
        const firstRow = firstLink.closest('li') as HTMLElement;
        const handles = await screen.findAllByRole('button', {name: /Drag to reorder/});
        const dataTransfer = {
            effectAllowed: '',
            dropEffect: '',
            setData: jest.fn(),
        };

        fireEvent.dragStart(handles[1], {dataTransfer});
        fireEvent.dragOver(firstRow, {clientY: 0, dataTransfer});
        fireEvent.drop(firstRow, {clientY: 0, dataTransfer});
        await waitFor(() => expect(reorderSavedLinksMock).toHaveBeenCalledTimes(1));
        await waitFor(() => expect(screen.getAllByRole('link').map((link) => link.textContent)).toEqual(['Design', 'Runbook']));

        // Drag Runbook back above Design.
        const rows = screen.getAllByRole('link').map((link) => link.closest('li') as HTMLElement);
        const handlesAfterMove = screen.getAllByRole('button', {name: /Drag to reorder/});
        fireEvent.dragStart(handlesAfterMove[1], {dataTransfer});
        fireEvent.dragOver(rows[0], {clientY: 0, dataTransfer});
        fireEvent.drop(rows[0], {clientY: 0, dataTransfer});

        await waitFor(() => expect(reorderSavedLinksMock).toHaveBeenCalledTimes(2));
        expect(reorderSavedLinksMock).toHaveBeenLastCalledWith(['link-1', 'link-2']);
    });

    it('reorders saved links by dragging a row', async () => {
        fetchSavedLinksMock.mockResolvedValue(page([savedLink, secondSavedLink]));
        renderPanel();
        const firstLink = await screen.findByRole('link', {name: 'Runbook'});
        const firstRow = firstLink.closest('li') as HTMLElement;

        const handles = await screen.findAllByRole('button', {name: /Drag to reorder/});
        const dataTransfer = {
            effectAllowed: '',
            dropEffect: '',
            setData: jest.fn(),
        };

        // Drag the second saved link above the first one.
        fireEvent.dragStart(handles[1], {dataTransfer});
        fireEvent.dragOver(firstRow, {clientY: 0, dataTransfer});
        expect(dataTransfer.dropEffect).toBe('move');
        fireEvent.drop(firstRow, {clientY: 0, dataTransfer});

        await waitFor(() => expect(reorderSavedLinksMock).toHaveBeenCalledWith(['link-2', 'link-1']));
    });

    it('edits and deletes a saved link', async () => {
        const user = userEvent.setup();
        const onEdit = jest.fn();
        const onDeleted = jest.fn();
        renderPanel({onEdit, onDeleted});
        await screen.findByRole('link', {name: 'Runbook'});

        await user.click(screen.getByRole('button', {name: 'Edit saved link'}));
        expect(onEdit).toHaveBeenCalledWith('link-1');

        await user.click(screen.getByRole('button', {name: 'Delete saved link'}));
        await user.click(screen.getByRole('button', {name: 'Delete'}));

        await waitFor(() => expect(deleteSavedLinkMock).toHaveBeenCalledWith('link-1'));
        expect(onDeleted).toHaveBeenCalled();
    });

    it('shows the empty state', async () => {
        fetchSavedLinksMock.mockResolvedValue(page([]));
        renderPanel();

        expect(await screen.findByText('No saved links yet')).toBeVisible();
    });

    it('shows a load error', async () => {
        fetchSavedLinksMock.mockRejectedValueOnce(new Error('Network down'));
        renderPanel();

        expect(await screen.findByText('Couldn’t load saved links')).toBeVisible();
        expect(screen.getByText('Network down')).toBeVisible();
    });
});
