// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {createSavedLink, updateSavedLink} from 'client';
import {useFolders} from 'hooks';
import {useEffect, useState} from 'react';
import {FormattedMessage, useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import {Button} from '@mattermost/compass-ui/components/button';
import {SectionNotice} from '@mattermost/compass-ui/components/section-notice';
import {Select} from '@mattermost/compass-ui/components/select';
import {TextInput} from '@mattermost/compass-ui/components/text-input';

import {selectCurrentTeam, selectMyChannels, selectMyTeams} from 'selectors';

import Dialog from 'components/dialog/dialog';
import {TEAM_LEVEL} from 'components/move_dialog/move_dialog';

import type {SavedLink} from 'types';

import './saved_link_editor.css';

const NO_FOLDER = 'none';

export type SavedLinkEditorDefaults = {
    teamId?: string;
    channelId?: string;
    folderId?: string;
    url?: string;
    urls?: string[];
    name?: string;
    tags?: string[];
    sourcePostId?: string;
};

type Props = {
    savedLink?: SavedLink;
    defaults?: SavedLinkEditorDefaults;
    onClose: () => void;
    onSaved: (savedLink: SavedLink) => void;
};

export default function SavedLinkEditor({savedLink, defaults, onClose, onSaved}: Props) {
    const intl = useIntl();
    const currentTeam = useSelector(selectCurrentTeam);
    const myTeams = useSelector(selectMyTeams);
    const myChannels = useSelector(selectMyChannels);

    const urls = defaults?.urls ?? [];
    const initialTeamID = savedLink?.teamId ?? defaults?.teamId ?? currentTeam?.id ?? myTeams[0]?.id ?? '';
    const initialFolderID = savedLink?.folderId ?? defaults?.folderId ?? '';
    const [teamID, setTeamID] = useState(initialTeamID);
    const [channelID, setChannelID] = useState(savedLink?.channelId ?? defaults?.channelId ?? '');
    const [folderID, setFolderID] = useState(initialFolderID || NO_FOLDER);
    const [url, setURL] = useState(savedLink?.url ?? defaults?.url ?? urls[0] ?? '');
    const [name, setName] = useState(savedLink?.name ?? defaults?.name ?? '');
    const [tags, setTags] = useState((savedLink?.tags ?? defaults?.tags ?? []).join(', '));
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string>();

    const foldersState = useFolders(teamID || undefined, channelID || undefined, 0);
    const folders = foldersState.data?.items ?? [];

    // If the saved link's folder was deleted while it was open, fall back to no folder.
    useEffect(() => {
        if (folderID === NO_FOLDER || foldersState.loading || !foldersState.data) {
            return;
        }
        if (!foldersState.data.items.some((folder) => folder.id === folderID)) {
            setFolderID(NO_FOLDER);
        }
    }, [folderID, foldersState.data, foldersState.loading]);

    const teamOptions = myTeams.map((team) => ({value: team.id, label: team.display_name}));
    if (teamID && !teamOptions.some((option) => option.value === teamID)) {
        teamOptions.push({value: teamID, label: teamID});
    }
    const channelOptions = myChannels.
        filter((channel) => channel.team_id === teamID && (channel.type === 'O' || channel.type === 'P')).
        map((channel) => ({value: channel.id, label: channel.display_name}));
    channelOptions.unshift({value: TEAM_LEVEL, label: intl.formatMessage({id: 'savedLinks.editor.team_level', defaultMessage: 'Team-level (no channel)'})});
    if (channelID && !channelOptions.some((option) => option.value === channelID)) {
        channelOptions.push({value: channelID, label: intl.formatMessage({id: 'savedLinks.editor.current_channel', defaultMessage: 'Current channel'})});
    }
    const urlOptions = urls.length > 1 ? urls.map((candidate) => ({value: candidate, label: candidate})) : [];

    const folderOptions = [
        {value: NO_FOLDER, label: intl.formatMessage({id: 'savedLinks.editor.no_folder', defaultMessage: 'No folder'})},
        ...folders.map((folder) => ({value: folder.id, label: folder.name})),
    ];
    if (folderID !== NO_FOLDER && !folderOptions.some((option) => option.value === folderID)) {
        folderOptions.push({value: folderID, label: intl.formatMessage({id: 'savedLinks.editor.current_folder', defaultMessage: 'Current folder'})});
    }

    const save = async () => {
        const trimmedURL = url.trim();
        const trimmedName = name.trim();
        if (!trimmedURL) {
            setError(intl.formatMessage({id: 'savedLinks.link.url_required', defaultMessage: 'Enter a URL.'}));
            return;
        }
        const tagList = tags.split(',').map((tag) => tag.trim()).filter(Boolean);
        const folderId = folderID === NO_FOLDER ? '' : folderID;

        setSaving(true);
        setError(undefined);
        try {
            if (savedLink) {
                const updated = await updateSavedLink(savedLink.id, {
                    teamId: teamID,
                    channelId: channelID,
                    folderId,
                    url: trimmedURL,
                    name: trimmedName,
                    tags: tagList,
                });
                onSaved(updated);
            } else {
                const created = await createSavedLink({
                    teamId: teamID || undefined,
                    channelId: channelID || undefined,
                    folderId: folderId || undefined,
                    url: trimmedURL,
                    name: trimmedName || undefined,
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
                    id='savedLinks.cancel'
                    defaultMessage='Cancel'
                />
            </Button>
            <Button
                emphasis='primary'
                onClick={save}
                loading={saving}
            >
                {savedLink ? (
                    <FormattedMessage
                        id='savedLinks.save'
                        defaultMessage='Save'
                    />
                ) : (
                    <FormattedMessage
                        id='savedLinks.link.create'
                        defaultMessage='Save link'
                    />
                )}
            </Button>
        </>
    );

    return (
        <Dialog
            title={savedLink ? (
                <FormattedMessage
                    id='savedLinks.link.edit_title'
                    defaultMessage='Edit saved link'
                />
            ) : (
                <FormattedMessage
                    id='savedLinks.link.create_title'
                    defaultMessage='New saved link'
                />
            )}
            onClose={onClose}
            footer={footer}
        >
            <div className='SavedLinks-Editor'>
                {error && (
                    <SectionNotice
                        type='danger'
                        title={error}
                    />
                )}
                {urlOptions.length > 0 && (
                    <Select
                        label={intl.formatMessage({id: 'savedLinks.link.choose_url', defaultMessage: 'URL from the post'})}
                        options={urlOptions}
                        value={url}
                        onChange={setURL}
                    />
                )}
                <TextInput
                    label={intl.formatMessage({id: 'savedLinks.link.url', defaultMessage: 'URL'})}
                    value={url}
                    onChange={(event) => setURL(event.target.value)}
                />
                <TextInput
                    label={intl.formatMessage({id: 'savedLinks.link.name', defaultMessage: 'Name'})}
                    value={name}
                    maxLength={200}
                    onChange={(event) => setName(event.target.value)}
                />
                <p className='SavedLinks-Editor__hint'>
                    <FormattedMessage
                        id='savedLinks.link.name_hint'
                        defaultMessage='Optional. If you leave the name empty, it is derived from the URL.'
                    />
                </p>
                <TextInput
                    label={intl.formatMessage({id: 'savedLinks.link.tags', defaultMessage: 'Tags (comma separated)'})}
                    value={tags}
                    onChange={(event) => setTags(event.target.value)}
                />
                {(savedLink?.sourcePostId || defaults?.sourcePostId) && (
                    <p className='SavedLinks-Editor__hint'>
                        <FormattedMessage
                            id='savedLinks.link.source_scope_fixed'
                            defaultMessage='This saved link comes from a post, so its team and channel are fixed.'
                        />
                    </p>
                )}
                <div className='SavedLinks-Editor__row'>
                    <Select
                        label={intl.formatMessage({id: 'savedLinks.editor.team', defaultMessage: 'Team'})}
                        options={teamOptions}
                        value={teamID}
                        disabled={Boolean(savedLink?.sourcePostId || defaults?.sourcePostId)}
                        onChange={(value) => {
                            setTeamID(value);
                            setChannelID('');
                            setFolderID(NO_FOLDER);
                        }}
                    />
                    <Select
                        label={intl.formatMessage({id: 'savedLinks.editor.channel', defaultMessage: 'Scope'})}
                        options={channelOptions}
                        value={channelID || TEAM_LEVEL}
                        disabled={Boolean(savedLink?.sourcePostId || defaults?.sourcePostId)}
                        onChange={(value) => {
                            setChannelID(value === TEAM_LEVEL ? '' : value);
                            setFolderID(NO_FOLDER);
                        }}
                    />
                </div>
                <Select
                    label={intl.formatMessage({id: 'savedLinks.editor.folder', defaultMessage: 'Folder'})}
                    options={folderOptions}
                    value={folderID}
                    onChange={setFolderID}
                />
                {foldersState.error && (
                    <p className='SavedLinks-Editor__hint'>
                        <FormattedMessage
                            id='savedLinks.editor.folders_error'
                            defaultMessage='Couldn’t load folders. You can still save without one.'
                        />
                    </p>
                )}
            </div>
        </Dialog>
    );
}
