// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {createFolder, updateFolder} from 'client';
import {useState} from 'react';
import {FormattedMessage, useIntl} from 'react-intl';

import {Button} from '@mattermost/compass-ui/components/button';
import {SectionNotice} from '@mattermost/compass-ui/components/section-notice';
import {TextInput} from '@mattermost/compass-ui/components/text-input';

import Dialog from 'components/dialog/dialog';

import type {Folder, FolderSummary} from 'types';

import './folder_editor.css';

type Props = {
    teamId?: string;
    channelId?: string;
    folder?: FolderSummary;
    onClose: () => void;
    onSaved: (folder: Folder) => void;
};

export default function FolderEditor({teamId, channelId, folder, onClose, onSaved}: Props) {
    const intl = useIntl();
    const [name, setName] = useState(folder?.name ?? '');
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string>();

    const save = async () => {
        if (saving) {
            return;
        }
        const trimmed = name.trim();
        if (!trimmed) {
            setError(intl.formatMessage({id: 'savedLinks.folder.name_required', defaultMessage: 'Enter a folder name.'}));
            return;
        }

        setSaving(true);
        setError(undefined);
        try {
            const saved = folder ?
                await updateFolder(folder.id, {name: trimmed}) :
                await createFolder({
                    teamId: channelId ? undefined : teamId,
                    channelId: channelId || undefined,
                    name: trimmed,
                });
            onSaved(saved);
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
                {folder ? (
                    <FormattedMessage
                        id='savedLinks.folder.rename'
                        defaultMessage='Rename folder'
                    />
                ) : (
                    <FormattedMessage
                        id='savedLinks.folder.create'
                        defaultMessage='Create folder'
                    />
                )}
            </Button>
        </>
    );

    return (
        <Dialog
            title={folder ? (
                <FormattedMessage
                    id='savedLinks.folder.rename_title'
                    defaultMessage='Rename folder'
                />
            ) : (
                <FormattedMessage
                    id='savedLinks.folder.create_title'
                    defaultMessage='New folder'
                />
            )}
            onClose={onClose}
            footer={footer}
        >
            <div className='SavedLinks-FolderEditor'>
                {error && (
                    <SectionNotice
                        type='danger'
                        title={error}
                    />
                )}
                <TextInput
                    label={intl.formatMessage({id: 'savedLinks.folder.name', defaultMessage: 'Folder name'})}
                    value={name}
                    maxLength={100}
                    autoFocus={true}
                    onChange={(event) => setName(event.target.value)}
                    onKeyDown={(event) => {
                        if (event.key === 'Enter') {
                            event.preventDefault();
                            save();
                        }
                    }}
                />
            </div>
        </Dialog>
    );
}
