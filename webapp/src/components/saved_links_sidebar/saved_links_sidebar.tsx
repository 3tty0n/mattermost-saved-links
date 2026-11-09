// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {getSavedLink} from 'client';
import {useEffect, useState} from 'react';
import {useDispatch, useSelector} from 'react-redux';
import {clearComposer, getSavedLinksRevision, getComposer, remoteEvent} from 'state';

import {SectionNotice} from '@mattermost/compass-ui/components/section-notice';

import {selectCurrentChannel, selectCurrentTeam} from 'selectors';

import SavedLinkEditor, {type SavedLinkEditorDefaults} from 'components/saved_link_editor/saved_link_editor';
import SavedLinkPanel from 'components/saved_link_panel/saved_link_panel';

import type {SavedLink, SavedLinkScope} from 'types';

import './saved_links_sidebar.css';

type EditorState = {savedLink?: SavedLink; defaults?: SavedLinkEditorDefaults};

function isTeamChannel(channel: {type?: string; team_id?: string} | undefined) {
    return Boolean(channel && channel.team_id && (channel.type === 'O' || channel.type === 'P'));
}

// SavedLinksSidebar is its own Mattermost app panel, with its own channel header entry point and
// RHS. It intentionally shares no state with other panels other than the plugin's revision
// counters, so opening one does not disturb the other.
export default function SavedLinksSidebar() {
    const dispatch = useDispatch();
    const currentTeam = useSelector(selectCurrentTeam);
    const currentChannel = useSelector(selectCurrentChannel);
    const revision = useSelector(getSavedLinksRevision);
    const composer = useSelector(getComposer);

    const [editor, setEditor] = useState<EditorState>();
    const [actionError, setActionError] = useState<string>();

    const teamID = currentTeam?.id;
    const channelID = currentChannel && isTeamChannel(currentChannel) ? currentChannel.id : undefined;

    useEffect(() => {
        if (!composer || composer.kind !== 'saved-link') {
            return;
        }
        dispatch(clearComposer());
        setEditor({
            defaults: {
                teamId: teamID,
                channelId: composer.channelId ?? channelID,
                folderId: composer.folderId,
                urls: composer.urls,
                url: composer.urls?.[0],
                name: composer.name,
                sourcePostId: composer.postId,
            },
        });

        // The composer is consumed synchronously; only the current scope values are needed.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [composer]);

    const refresh = () => dispatch(remoteEvent());

    const editSavedLink = async (savedLinkID: string) => {
        setActionError(undefined);
        try {
            setEditor({savedLink: await getSavedLink(savedLinkID)});
        } catch (err) {
            setActionError(err instanceof Error ? err.message : String(err));
        }
    };

    const createSavedLink = (scope: SavedLinkScope, folderId?: string) => {
        setEditor({
            defaults: {
                teamId: teamID,
                channelId: scope === 'channel' ? channelID : undefined,
                folderId,
            },
        });
    };

    return (
        <div className='SavedLinks-Sidebar'>
            {actionError && (
                <div className='SavedLinks-Sidebar__notice'>
                    <SectionNotice
                        type='danger'
                        title={actionError}
                        onDismiss={() => setActionError(undefined)}
                    />
                </div>
            )}
            <SavedLinkPanel
                teamId={teamID}
                channelId={channelID}
                revision={revision}
                onCreate={createSavedLink}
                onEdit={editSavedLink}
                onDeleted={refresh}
            />
            {editor && (
                <SavedLinkEditor
                    savedLink={editor.savedLink}
                    defaults={editor.defaults}
                    onClose={() => setEditor(undefined)}
                    onSaved={() => {
                        setEditor(undefined);
                        refresh();
                    }}
                />
            )}
        </div>
    );
}
