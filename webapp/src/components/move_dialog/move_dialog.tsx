// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {useState, type ReactNode} from 'react';
import {FormattedMessage, useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import {Button} from '@mattermost/compass-ui/components/button';
import {SectionNotice} from '@mattermost/compass-ui/components/section-notice';
import {Select} from '@mattermost/compass-ui/components/select';

import {selectMyChannels, selectMyTeams} from 'selectors';

import Dialog from 'components/dialog/dialog';

import '../saved_link_editor/saved_link_editor.css';

// compass Select drops options whose value is empty, so team-level uses this stand-in value.
export const TEAM_LEVEL = 'team-level';

type Props = {
    title: ReactNode;
    teamId: string;
    channelId?: string;
    hint?: ReactNode;
    onMove: (teamId: string, channelId: string) => Promise<void>;
    onClose: () => void;
};

// Picks a new team or channel for a saved link or a folder.
export default function MoveDialog({title, teamId, channelId, hint, onMove, onClose}: Props) {
    const intl = useIntl();
    const myTeams = useSelector(selectMyTeams);
    const myChannels = useSelector(selectMyChannels);
    const [team, setTeam] = useState(teamId);
    const [channel, setChannel] = useState(channelId ?? '');
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string>();

    const teamOptions = myTeams.map((item) => ({value: item.id, label: item.display_name}));
    if (!teamOptions.some((option) => option.value === team)) {
        teamOptions.push({value: team, label: team});
    }
    const channelOptions = [
        {value: TEAM_LEVEL, label: intl.formatMessage({id: 'savedLinks.editor.team_level', defaultMessage: 'Team-level (no channel)'})},
        ...myChannels.
            filter((item) => item.team_id === team && (item.type === 'O' || item.type === 'P')).
            map((item) => ({value: item.id, label: item.display_name})).
            sort((a, b) => a.label.localeCompare(b.label)),
    ];
    if (channel && !channelOptions.some((option) => option.value === channel)) {
        channelOptions.push({value: channel, label: intl.formatMessage({id: 'savedLinks.editor.current_channel', defaultMessage: 'Current channel'})});
    }

    const unchanged = team === teamId && channel === (channelId ?? '');

    const move = async () => {
        setSaving(true);
        setError(undefined);
        try {
            await onMove(team, channel);
        } catch (err) {
            setSaving(false);
            setError(err instanceof Error ? err.message : String(err));
        }
    };

    return (
        <Dialog
            title={title}
            onClose={onClose}
            footer={(
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
                        onClick={move}
                        loading={saving}
                        disabled={unchanged}
                    >
                        <FormattedMessage
                            id='savedLinks.move.confirm'
                            defaultMessage='Move'
                        />
                    </Button>
                </>
            )}
        >
            <div className='SavedLinks-Editor'>
                {error && (
                    <SectionNotice
                        type='danger'
                        title={error}
                    />
                )}
                {hint && <p className='SavedLinks-Editor__hint'>{hint}</p>}
                <div className='SavedLinks-Editor__row'>
                    <Select
                        label={intl.formatMessage({id: 'savedLinks.editor.team', defaultMessage: 'Team'})}
                        options={teamOptions}
                        value={team}
                        onChange={(value) => {
                            setTeam(value);
                            setChannel('');
                        }}
                    />
                    <Select
                        label={intl.formatMessage({id: 'savedLinks.editor.channel', defaultMessage: 'Scope'})}
                        options={channelOptions}
                        value={channel || TEAM_LEVEL}
                        onChange={(value) => setChannel(value === TEAM_LEVEL ? '' : value)}
                    />
                </div>
            </div>
        </Dialog>
    );
}
