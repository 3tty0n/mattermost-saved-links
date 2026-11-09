// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {deleteBookmark, getSitePath} from 'client';
import {useBookmarks, useDebouncedValue} from 'hooks';
import {useEffect, useState} from 'react';
import {FormattedMessage, useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import PencilOutlineIcon from '@mattermost/compass-icons/components/pencil-outline';
import PlusIcon from '@mattermost/compass-icons/components/plus';
import TrashCanOutlineIcon from '@mattermost/compass-icons/components/trash-can-outline';
import {Button} from '@mattermost/compass-ui/components/button';
import {EmptyState} from '@mattermost/compass-ui/components/empty-state';
import {Icon} from '@mattermost/compass-ui/components/icon';
import {IconButton} from '@mattermost/compass-ui/components/icon-button';
import {SearchInput} from '@mattermost/compass-ui/components/search-input';
import {SectionNotice} from '@mattermost/compass-ui/components/section-notice';
import {Spinner} from '@mattermost/compass-ui/components/spinner';
import {Tabs} from '@mattermost/compass-ui/components/tabs';
import {Tag} from '@mattermost/compass-ui/components/tag';

import {selectAllChannels, selectTeams} from 'selectors';

import type {BookmarkScope, BookmarkSummary} from 'types';

import './bookmark_panel.css';

type Props = {
    teamId?: string;
    channelId?: string;
    revision: number;
    onCreate: (scope: BookmarkScope) => void;
    onEdit: (bookmarkId: string) => void;
    onDeleted: () => void;
};

function defaultBookmarkScope(channelId: string | undefined, teamId: string | undefined): BookmarkScope {
    if (channelId) {
        return 'channel';
    }
    if (teamId) {
        return 'team';
    }
    return 'all';
}

function safeHttpUrl(value: string): string | undefined {
    try {
        const parsed = new URL(value);
        return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.toString() : undefined;
    } catch {
        return undefined;
    }
}

export default function BookmarkPanel({teamId, channelId, revision, onCreate, onEdit, onDeleted}: Props) {
    const intl = useIntl();
    const channels = useSelector(selectAllChannels);
    const teams = useSelector(selectTeams);
    const [scope, setScope] = useState<BookmarkScope>(defaultBookmarkScope(channelId, teamId));
    const [search, setSearch] = useState('');
    const debouncedSearch = useDebouncedValue(search, 250);
    const [page, setPage] = useState(0);
    const [reload, setReload] = useState(0);
    const [confirmDelete, setConfirmDelete] = useState<string>();
    const [deleting, setDeleting] = useState(false);
    const [actionError, setActionError] = useState<string>();

    useEffect(() => {
        if (scope === 'channel' && !channelId) {
            setScope(teamId ? 'team' : 'all');
        } else if (scope === 'team' && !teamId) {
            setScope('all');
        }
    }, [scope, channelId, teamId]);

    const query = (scope === 'channel' && channelId) || (scope === 'team' && teamId) || scope === 'all' ? {
        scope,
        teamId: scope === 'channel' && channelId ? undefined : teamId,
        channelId: scope === 'channel' ? channelId : undefined,
        search: debouncedSearch,
        page,
        perPage: 20,
    } : null;
    const {data, loading, error} = useBookmarks(query, revision + reload);

    useEffect(() => {
        setPage(0);
    }, [scope, debouncedSearch, teamId, channelId]);

    useEffect(() => {
        if (data && data.items.length === 0 && data.total > 0 && page > 0) {
            setPage((value) => Math.max(0, value - 1));
        }
    }, [data, page]);

    const scopeOptions = [
        {
            key: 'channel',
            label: (
                <FormattedMessage
                    id='bookmarks.scope.channel'
                    defaultMessage='Channel'
                />
            ),
        },
        {
            key: 'team',
            label: (
                <FormattedMessage
                    id='bookmarks.scope.team'
                    defaultMessage='Team'
                />
            ),
        },
        {
            key: 'all',
            label: (
                <FormattedMessage
                    id='bookmarks.scope.all'
                    defaultMessage='All'
                />
            ),
        },
    ];

    const labelFor = (bookmark: BookmarkSummary) => {
        if (bookmark.channelId) {
            return channels[bookmark.channelId]?.display_name ?? bookmark.channelId;
        }
        return teams[bookmark.teamId]?.display_name ?? bookmark.teamId;
    };

    const remove = async (bookmarkId: string) => {
        setDeleting(true);
        try {
            await deleteBookmark(bookmarkId);
            setConfirmDelete(undefined);
            onDeleted();
        } catch (err) {
            setConfirmDelete(undefined);
            setActionError(err instanceof Error ? err.message : String(err));
            setReload((value) => value + 1);
        } finally {
            setDeleting(false);
        }
    };

    return (
        <div className='Bookmarks-BookmarkPanel'>
            <div className='Bookmarks-BookmarkPanel__toolbar'>
                <Tabs
                    tabs={scopeOptions}
                    activeKey={scope}
                    onChange={(key) => setScope(key as BookmarkScope)}
                    appearance='underlined'
                />
                <IconButton
                    icon={<Icon glyph={<PlusIcon/>}/>}
                    aria-label={intl.formatMessage({id: 'bookmarks.bookmark.new', defaultMessage: 'New bookmark'})}
                    onClick={() => onCreate(scope)}
                />
            </div>
            <div className='Bookmarks-BookmarkPanel__search'>
                <SearchInput
                    label={intl.formatMessage({id: 'bookmarks.bookmark.search', defaultMessage: 'Search bookmarks'})}
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    onClear={() => setSearch('')}
                    clearLabel={intl.formatMessage({id: 'bookmarks.search.clear', defaultMessage: 'Clear search'})}
                />
            </div>

            {actionError && (
                <SectionNotice
                    type='danger'
                    title={intl.formatMessage({id: 'bookmarks.bookmark.delete_error', defaultMessage: 'Couldn’t delete the bookmark'})}
                    description={actionError}
                    onDismiss={() => setActionError(undefined)}
                />
            )}
            {loading && (
                <div className='Bookmarks-BookmarkPanel__centered'>
                    <Spinner aria-label={intl.formatMessage({id: 'bookmarks.loading', defaultMessage: 'Loading…'})}/>
                </div>
            )}
            {error && !loading && (
                <SectionNotice
                    type='danger'
                    title={intl.formatMessage({id: 'bookmarks.bookmark.load_error', defaultMessage: 'Couldn’t load bookmarks'})}
                    description={error}
                    primaryButtonLabel={intl.formatMessage({id: 'bookmarks.retry', defaultMessage: 'Try again'})}
                    onPrimaryAction={() => setReload((value) => value + 1)}
                />
            )}
            {!loading && !error && data && data.items.length === 0 && (
                <EmptyState
                    title={intl.formatMessage({id: 'bookmarks.bookmark.empty', defaultMessage: 'No bookmarks yet'})}
                    description={intl.formatMessage({id: 'bookmarks.bookmark.empty_description', defaultMessage: 'Save a URL with the post menu or create one here.'})}
                    action={{
                        children: intl.formatMessage({id: 'bookmarks.bookmark.new', defaultMessage: 'New bookmark'}),
                        emphasis: 'primary',
                        size: 'small',
                        onClick: () => onCreate(scope),
                    }}
                />
            )}
            {!loading && !error && data && data.items.length > 0 && (
                <ul
                    className='Bookmarks-BookmarkPanel__list'
                    aria-live='polite'
                >
                    {data.items.map((bookmark) => {
                        const href = safeHttpUrl(bookmark.url);
                        return (
                            <li
                                key={bookmark.id}
                                className='Bookmarks-BookmarkPanel__item'
                            >
                                <div className='Bookmarks-BookmarkPanel__itemMain'>
                                    {href ? (
                                        // eslint-disable-next-line @mattermost/use-external-link -- plugin bundles cannot import host internals
                                        <a
                                            className='Bookmarks-BookmarkPanel__title'
                                            href={href}
                                            target='_blank'
                                            rel='noreferrer noopener'
                                        >
                                            {bookmark.title}
                                        </a>
                                    ) : (
                                        <span className='Bookmarks-BookmarkPanel__title'>{bookmark.title}</span>
                                    )}
                                    <span className='Bookmarks-BookmarkPanel__url'>{bookmark.url}</span>
                                    <div className='Bookmarks-BookmarkPanel__meta'>
                                        <span className='Bookmarks-BookmarkPanel__scope'>{labelFor(bookmark)}</span>
                                        {bookmark.tags.map((tag) => (
                                            <Tag
                                                key={tag}
                                                label={tag}
                                                size='x-small'
                                            />
                                        ))}
                                    </div>
                                    {bookmark.sourcePostId && teams[bookmark.teamId] && (
                                        <span className='Bookmarks-BookmarkPanel__source'>
                                            <a
                                                className='Bookmarks-BookmarkPanel__sourceLink'
                                                href={`${getSitePath()}/${teams[bookmark.teamId].name}/pl/${bookmark.sourcePostId}`}
                                            >
                                                <FormattedMessage
                                                    id='bookmarks.bookmark.open_post'
                                                    defaultMessage='Open the original post'
                                                />
                                            </a>
                                            {' · '}
                                            <FormattedMessage
                                                id='bookmarks.bookmark.from_post'
                                                defaultMessage='Saved from a post'
                                            />
                                        </span>
                                    )}
                                </div>
                                <div className='Bookmarks-BookmarkPanel__actions'>
                                    {confirmDelete === bookmark.id ? (
                                        <>
                                            <Button
                                                emphasis='tertiary'
                                                size='small'
                                                onClick={() => setConfirmDelete(undefined)}
                                                disabled={deleting}
                                            >
                                                <FormattedMessage
                                                    id='bookmarks.cancel'
                                                    defaultMessage='Cancel'
                                                />
                                            </Button>
                                            <Button
                                                emphasis='primary'
                                                size='small'
                                                destructive={true}
                                                loading={deleting}
                                                onClick={() => remove(bookmark.id)}
                                            >
                                                <FormattedMessage
                                                    id='bookmarks.bookmark.delete_confirm'
                                                    defaultMessage='Delete'
                                                />
                                            </Button>
                                        </>
                                    ) : (
                                        <>
                                            <IconButton
                                                size='small'
                                                icon={<Icon glyph={<PencilOutlineIcon/>}/>}
                                                aria-label={intl.formatMessage({id: 'bookmarks.bookmark.edit', defaultMessage: 'Edit bookmark'})}
                                                onClick={() => onEdit(bookmark.id)}
                                            />
                                            <IconButton
                                                size='small'
                                                icon={<Icon glyph={<TrashCanOutlineIcon/>}/>}
                                                aria-label={intl.formatMessage({id: 'bookmarks.bookmark.delete', defaultMessage: 'Delete bookmark'})}
                                                onClick={() => setConfirmDelete(bookmark.id)}
                                            />
                                        </>
                                    )}
                                </div>
                            </li>
                        );
                    })}
                </ul>
            )}

            {data && (data.hasMore || page > 0) && (
                <div className='Bookmarks-BookmarkPanel__pagination'>
                    <Button
                        emphasis='tertiary'
                        size='small'
                        disabled={page === 0}
                        onClick={() => setPage((value) => Math.max(0, value - 1))}
                    >
                        <FormattedMessage
                            id='bookmarks.pagination.previous'
                            defaultMessage='Previous'
                        />
                    </Button>
                    <Button
                        emphasis='tertiary'
                        size='small'
                        disabled={!data.hasMore}
                        onClick={() => setPage((value) => value + 1)}
                    >
                        <FormattedMessage
                            id='bookmarks.pagination.next'
                            defaultMessage='Next'
                        />
                    </Button>
                </div>
            )}
        </div>
    );
}
