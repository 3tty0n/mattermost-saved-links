// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {getBookmark} from 'client';
import {useEffect, useState} from 'react';
import {useDispatch, useSelector} from 'react-redux';
import {clearComposer, getBookmarksRevision, getComposer, remoteEvent} from 'state';

import {SectionNotice} from '@mattermost/compass-ui/components/section-notice';

import {selectCurrentChannel, selectCurrentTeam} from 'selectors';

import BookmarkEditor, {type BookmarkEditorDefaults} from 'components/bookmark_editor/bookmark_editor';
import BookmarkPanel from 'components/bookmark_panel/bookmark_panel';

import type {Bookmark, BookmarkScope} from 'types';

import './bookmarks_sidebar.css';

type EditorState = {bookmark?: Bookmark; defaults?: BookmarkEditorDefaults};

function isTeamChannel(channel: {type?: string; team_id?: string} | undefined) {
    return Boolean(channel && channel.team_id && (channel.type === 'O' || channel.type === 'P'));
}

// BookmarksSidebar is its own Mattermost app panel, with its own channel header entry point and
// RHS. It intentionally shares no state with the Tasks panel other than the plugin's revision
// counters, so opening one does not disturb the other.
export default function BookmarksSidebar() {
    const dispatch = useDispatch();
    const currentTeam = useSelector(selectCurrentTeam);
    const currentChannel = useSelector(selectCurrentChannel);
    const revision = useSelector(getBookmarksRevision);
    const composer = useSelector(getComposer);

    const [editor, setEditor] = useState<EditorState>();
    const [actionError, setActionError] = useState<string>();

    const teamID = currentTeam?.id;
    const channelID = currentChannel && isTeamChannel(currentChannel) ? currentChannel.id : undefined;

    useEffect(() => {
        if (!composer || composer.kind !== 'bookmark') {
            return;
        }
        dispatch(clearComposer());
        setEditor({
            defaults: {
                teamId: teamID,
                channelId: composer.channelId ?? channelID,
                urls: composer.urls,
                url: composer.urls?.[0],
                title: composer.title,
                description: composer.description,
                sourcePostId: composer.postId,
            },
        });

        // The composer is consumed synchronously; only the current scope values are needed.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [composer]);

    const refresh = () => dispatch(remoteEvent());

    const editBookmark = async (bookmarkID: string) => {
        setActionError(undefined);
        try {
            setEditor({bookmark: await getBookmark(bookmarkID)});
        } catch (err) {
            setActionError(err instanceof Error ? err.message : String(err));
        }
    };

    const createBookmark = (scope: BookmarkScope) => {
        setEditor({
            defaults: {
                teamId: teamID,
                channelId: scope === 'channel' ? channelID : undefined,
            },
        });
    };

    return (
        <div className='Bookmarks-BookmarksSidebar'>
            {actionError && (
                <div className='Bookmarks-BookmarksSidebar__notice'>
                    <SectionNotice
                        type='danger'
                        title={actionError}
                        onDismiss={() => setActionError(undefined)}
                    />
                </div>
            )}
            <BookmarkPanel
                teamId={teamID}
                channelId={channelID}
                revision={revision}
                onCreate={createBookmark}
                onEdit={editBookmark}
                onDeleted={refresh}
            />
            {editor && (
                <BookmarkEditor
                    bookmark={editor.bookmark}
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
