// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import manifest from 'manifest';
import type {UnknownAction} from 'redux';

import type {GlobalState} from '@mattermost/types/store';

import type {ComposerRequest} from 'types';

const REMOTE_EVENT = `${manifest.id}/remote_event`;
const RECONNECTED = `${manifest.id}/reconnected`;
const OPEN_COMPOSER = `${manifest.id}/open_composer`;
const CLEAR_COMPOSER = `${manifest.id}/clear_composer`;

type OpenComposerAction = {type: typeof OPEN_COMPOSER; request: ComposerRequest};

export type SavedLinksState = {
    savedLinksRevision: number;
    composer?: ComposerRequest;
};

const initialState: SavedLinksState = {
    savedLinksRevision: 0,
};

export function remoteEvent(): UnknownAction {
    return {type: REMOTE_EVENT};
}

export function reconnected(): UnknownAction {
    return {type: RECONNECTED};
}

export function openComposer(request: ComposerRequest): OpenComposerAction {
    return {type: OPEN_COMPOSER, request};
}

export function clearComposer(): UnknownAction {
    return {type: CLEAR_COMPOSER};
}

export function reducer(state: SavedLinksState = initialState, action: UnknownAction): SavedLinksState {
    switch (action.type) {
    case REMOTE_EVENT:
    case RECONNECTED:
        return {...state, savedLinksRevision: state.savedLinksRevision + 1};
    case OPEN_COMPOSER:
        return {...state, composer: (action as OpenComposerAction).request};
    case CLEAR_COMPOSER:
        return {...state, composer: undefined};
    default:
        return state;
    }
}

export const pluginStateKey = `plugins-${manifest.id}`;

function pluginState(state: GlobalState): SavedLinksState | undefined {
    return (state as unknown as Record<string, SavedLinksState | undefined>)[pluginStateKey];
}

export function getSavedLinksRevision(state: GlobalState): number {
    return pluginState(state)?.savedLinksRevision ?? 0;
}

export function getComposer(state: GlobalState): ComposerRequest | undefined {
    return pluginState(state)?.composer;
}
