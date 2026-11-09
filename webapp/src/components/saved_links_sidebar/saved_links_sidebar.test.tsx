// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {screen} from '@testing-library/react';
import {fetchSavedLinks, fetchFolders} from 'client';

import type {SavedLinkSummary, ListResult} from 'types';

import SavedLinksSidebar from './saved_links_sidebar';

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

const page: ListResult<SavedLinkSummary> = {items: [savedLink], total: 1, page: 0, perPage: 20, hasMore: false};

describe('SavedLinksSidebar', () => {
    it('renders saved links in their own panel without task views', async () => {
        (fetchSavedLinks as jest.Mock).mockResolvedValue(page);
        (fetchFolders as jest.Mock).mockResolvedValue({items: [], total: 0});
        renderWithStore(<SavedLinksSidebar/>);

        expect(await screen.findByRole('link', {name: 'Runbook'})).toBeVisible();
        expect(screen.queryByRole('button', {name: 'New task'})).not.toBeInTheDocument();
    });
});
