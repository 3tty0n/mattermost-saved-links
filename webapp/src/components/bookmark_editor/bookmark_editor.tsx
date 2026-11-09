// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {createBookmark, updateBookmark} from 'client';
import {useState} from 'react';
import {FormattedMessage, useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import {Button} from '@mattermost/compass-ui/components/button';
import {SectionNotice} from '@mattermost/compass-ui/components/section-notice';
import {Select} from '@mattermost/compass-ui/components/select';
import {TextArea} from '@mattermost/compass-ui/components/text-area';
import {TextInput} from '@mattermost/compass-ui/components/text-input';

import {selectCurrentTeam, selectMyChannels, selectMyTeams} from 'selectors';

import Dialog from 'components/dialog/dialog';

import type {Bookmark} from 'types';

import './bookmark_editor.css';

export type BookmarkEditorDefaults = {
    teamId?: string;
    channelId?: string;
    url?: string;
    urls?: string[];
    title?: string;
    description?: string;
    tags?: string[];
    sourcePostId?: string;
};

type Props = {
    bookmark?: Bookmark;
    defaults?: BookmarkEditorDefaults;
    onClose: () => void;
    onSaved: (bookmark: Bookmark) => void;
};

export default function BookmarkEditor({bookmark, defaults, onClose, onSaved}: Props) {
    const intl = useIntl();
    const currentTeam = useSelector(selectCurrentTeam);
    const myTeams = useSelector(selectMyTeams);
    const myChannels = useSelector(selectMyChannels);

    const urls = defaults?.urls ?? [];
    const initialTeamID = bookmark?.teamId ?? defaults?.teamId ?? currentTeam?.id ?? myTeams[0]?.id ?? '';
    const [teamID, setTeamID] = useState(initialTeamID);
    const [channelID, setChannelID] = useState(bookmark?.channelId ?? defaults?.channelId ?? '');
    const [url, setURL] = useState(bookmark?.url ?? defaults?.url ?? urls[0] ?? '');
    const [title, setTitle] = useState(bookmark?.title ?? defaults?.title ?? '');
    const [description, setDescription] = useState(bookmark?.description ?? defaults?.description ?? '');
    const [tags, setTags] = useState((bookmark?.tags ?? defaults?.tags ?? []).join(', '));
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string>();

    const teamOptions = myTeams.map((team) => ({value: team.id, label: team.display_name}));
    if (teamID && !teamOptions.some((option) => option.value === teamID)) {
        teamOptions.push({value: teamID, label: teamID});
    }
    const channelOptions = myChannels.
        filter((channel) => channel.team_id === teamID && (channel.type === 'O' || channel.type === 'P')).
        map((channel) => ({value: channel.id, label: channel.display_name}));
    channelOptions.unshift({value: '', label: intl.formatMessage({id: 'bookmarks.editor.team_level', defaultMessage: 'Team-level (no channel)'})});
    if (channelID && !channelOptions.some((option) => option.value === channelID)) {
        channelOptions.push({value: channelID, label: intl.formatMessage({id: 'bookmarks.editor.current_channel', defaultMessage: 'Current channel'})});
    }
    const urlOptions = urls.length > 1 ? urls.map((candidate) => ({value: candidate, label: candidate})) : [];

    const save = async () => {
        const trimmedURL = url.trim();
        const trimmedTitle = title.trim();
        if (!trimmedURL) {
            setError(intl.formatMessage({id: 'bookmarks.bookmark.url_required', defaultMessage: 'Enter a URL.'}));
            return;
        }
        if (!trimmedTitle) {
            setError(intl.formatMessage({id: 'bookmarks.bookmark.title_required', defaultMessage: 'Enter a title.'}));
            return;
        }
        const tagList = tags.split(',').map((tag) => tag.trim()).filter(Boolean);
        setSaving(true);
        setError(undefined);
        try {
            if (bookmark) {
                const updated = await updateBookmark(bookmark.id, {
                    teamId: teamID,
                    channelId: channelID,
                    url: trimmedURL,
                    title: trimmedTitle,
                    description,
                    tags: tagList,
                });
                onSaved(updated);
            } else {
                const created = await createBookmark({
                    teamId: teamID || undefined,
                    channelId: channelID || undefined,
                    url: trimmedURL,
                    title: trimmedTitle,
                    description,
                    tags: tagList,
                    sourcePostId: defaults?.sourcePostId,
                });
                onSaved(created);
            }
        } catch (err) {
            setSaving(false);
            setError(err instanceof Error ? err.message : String(err));
        }
    };

    const footer = (
        <>
            <Button
                emphasis='tertiary'
                onClick={onClose}
                disabled={saving}
            >
                <FormattedMessage
                    id='bookmarks.cancel'
                    defaultMessage='Cancel'
                />
            </Button>
            <Button
                emphasis='primary'
                onClick={save}
                loading={saving}
            >
                {bookmark ? (
                    <FormattedMessage
                        id='bookmarks.save'
                        defaultMessage='Save'
                    />
                ) : (
                    <FormattedMessage
                        id='bookmarks.bookmark.create'
                        defaultMessage='Save bookmark'
                    />
                )}
            </Button>
        </>
    );

    return (
        <Dialog
            title={bookmark ? (
                <FormattedMessage
                    id='bookmarks.bookmark.edit_title'
                    defaultMessage='Edit bookmark'
                />
            ) : (
                <FormattedMessage
                    id='bookmarks.bookmark.create_title'
                    defaultMessage='New bookmark'
                />
            )}
            onClose={onClose}
            footer={footer}
        >
            <div className='Bookmarks-BookmarkEditor'>
                {error && (
                    <SectionNotice
                        type='danger'
                        title={error}
                    />
                )}
                {urlOptions.length > 0 && (
                    <Select
                        label={intl.formatMessage({id: 'bookmarks.bookmark.choose_url', defaultMessage: 'URL from the post'})}
                        options={urlOptions}
                        value={url}
                        onChange={setURL}
                    />
                )}
                <TextInput
                    label={intl.formatMessage({id: 'bookmarks.bookmark.url', defaultMessage: 'URL'})}
                    value={url}
                    onChange={(event) => setURL(event.target.value)}
                />
                <TextInput
                    label={intl.formatMessage({id: 'bookmarks.bookmark.title', defaultMessage: 'Title'})}
                    value={title}
                    maxLength={200}
                    onChange={(event) => setTitle(event.target.value)}
                />
                <TextArea
                    label={intl.formatMessage({id: 'bookmarks.bookmark.description', defaultMessage: 'Description'})}
                    value={description}
                    maxLength={8000}
                    rows={3}
                    onChange={(event) => setDescription(event.target.value)}
                />
                <TextInput
                    label={intl.formatMessage({id: 'bookmarks.bookmark.tags', defaultMessage: 'Tags (comma separated)'})}
                    value={tags}
                    onChange={(event) => setTags(event.target.value)}
                />
                {(bookmark?.sourcePostId || defaults?.sourcePostId) && (
                    <p className='Bookmarks-BookmarkEditor__hint'>
                        <FormattedMessage
                            id='bookmarks.bookmark.source_scope_fixed'
                            defaultMessage='This bookmark was saved from a post, so its team and channel are fixed.'
                        />
                    </p>
                )}
                <div className='Bookmarks-BookmarkEditor__row'>
                    <Select
                        label={intl.formatMessage({id: 'bookmarks.editor.team', defaultMessage: 'Team'})}
                        options={teamOptions}
                        value={teamID}
                        disabled={Boolean(bookmark?.sourcePostId || defaults?.sourcePostId)}
                        onChange={(value) => {
                            setTeamID(value);
                            setChannelID('');
                        }}
                    />
                    <Select
                        label={intl.formatMessage({id: 'bookmarks.editor.channel', defaultMessage: 'Scope'})}
                        options={channelOptions}
                        value={channelID}
                        disabled={Boolean(bookmark?.sourcePostId || defaults?.sourcePostId)}
                        onChange={setChannelID}
                    />
                </div>
            </div>
        </Dialog>
    );
}
