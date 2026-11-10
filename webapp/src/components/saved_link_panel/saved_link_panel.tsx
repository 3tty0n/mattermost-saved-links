// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {deleteSavedLink, deleteFolder, getSitePath, reorderSavedLinks, updateFolder, updateSavedLink} from 'client';
import {useSavedLinks, useDebouncedValue, useFolders} from 'hooks';
import {useEffect, useRef, useState, type DragEvent, type KeyboardEvent, type ReactNode} from 'react';
import {FormattedMessage, useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import ArrowLeftIcon from '@mattermost/compass-icons/components/arrow-left';
import ChevronRightIcon from '@mattermost/compass-icons/components/chevron-right';
import DragVerticalIcon from '@mattermost/compass-icons/components/drag-vertical';
import FolderMoveOutlineIcon from '@mattermost/compass-icons/components/folder-move-outline';
import FolderOutlineIcon from '@mattermost/compass-icons/components/folder-outline';
import FolderPlusOutlineIcon from '@mattermost/compass-icons/components/folder-plus-outline';
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

import Dialog from 'components/dialog/dialog';
import FolderEditor from 'components/folder_editor/folder_editor';
import MoveDialog from 'components/move_dialog/move_dialog';

import type {SavedLinkScope, SavedLinkSummary, Folder, FolderSummary} from 'types';

import './saved_link_panel.css';

type Props = {
    teamId?: string;
    channelId?: string;
    revision: number;
    onCreate: (scope: SavedLinkScope, folderId?: string) => void;
    onEdit: (savedLinkId: string) => void;
    onDeleted: () => void;
};

type DropTarget = {id: string; after: boolean};

function defaultSavedLinkScope(channelId: string | undefined, teamId: string | undefined): SavedLinkScope {
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

function savedLinkGroupKey(savedLink: SavedLinkSummary): string {
    return `${savedLink.teamId}:${savedLink.channelId ?? ''}:${savedLink.folderId ?? ''}`;
}

export default function SavedLinkPanel({teamId, channelId, revision, onCreate, onEdit, onDeleted}: Props) {
    const intl = useIntl();
    const channels = useSelector(selectAllChannels);
    const teams = useSelector(selectTeams);
    const [scope, setScope] = useState<SavedLinkScope>(defaultSavedLinkScope(channelId, teamId));
    const [search, setSearch] = useState('');
    const debouncedSearch = useDebouncedValue(search, 250);
    const searchActive = debouncedSearch.trim() !== '';
    const [page, setPage] = useState(0);
    const [reload, setReload] = useState(0);
    const [confirmDelete, setConfirmDelete] = useState<string>();
    const [deleting, setDeleting] = useState(false);
    const [actionError, setActionError] = useState<string>();
    const [sortError, setSortError] = useState<string>();
    const [currentFolder, setCurrentFolder] = useState<FolderSummary>();
    const [folderEditor, setFolderEditor] = useState<{folder?: FolderSummary}>();
    const [folderToDelete, setFolderToDelete] = useState<FolderSummary>();
    const [moving, setMoving] = useState<{savedLink?: SavedLinkSummary; folder?: FolderSummary}>();
    const [folderActionError, setFolderActionError] = useState<string>();
    const [deletingFolder, setDeletingFolder] = useState(false);
    const [draggedID, setDraggedID] = useState<string>();
    const [dropTarget, setDropTarget] = useState<DropTarget>();
    const [pendingItems, setPendingItems] = useState<SavedLinkSummary[] | null>(null);
    const reorderingRef = useRef(false);
    const queuedReorderRef = useRef<SavedLinkSummary[] | null>(null);

    useEffect(() => {
        if (scope === 'channel' && !channelId) {
            setScope(teamId ? 'team' : 'all');
        } else if (scope === 'team' && !teamId) {
            setScope('all');
        }
    }, [scope, channelId, teamId]);

    // Folders and their contents are tied to one scope, so switching tabs starts at the root.
    useEffect(() => {
        setCurrentFolder(undefined);
        setFolderEditor(undefined);
        setFolderToDelete(undefined);
        setMoving(undefined);
        setFolderActionError(undefined);
        setSortError(undefined);
        setDraggedID(undefined);
        setDropTarget(undefined);
    }, [scope, teamId, channelId]);

    const folderTeamId = scope === 'team' ? teamId : undefined;
    const folderChannelId = scope === 'channel' ? channelId : undefined;
    const showFolders = scope !== 'all' && Boolean(folderTeamId || folderChannelId);
    const foldersState = useFolders(folderTeamId, folderChannelId, revision + reload);
    const folders = foldersState.data?.items ?? [];

    // Refresh the open folder from the list so renames, deletions and counts stay current.
    useEffect(() => {
        if (!currentFolder || foldersState.loading || !foldersState.data) {
            return;
        }
        const latest = foldersState.data.items.find((folder) => folder.id === currentFolder.id);
        if (!latest) {
            setCurrentFolder(undefined);
            return;
        }
        if (latest.name !== currentFolder.name || latest.savedLinkCount !== currentFolder.savedLinkCount) {
            setCurrentFolder(latest);
        }
    }, [currentFolder, foldersState.data, foldersState.loading]);

    const currentFolderID = currentFolder?.id;
    const folderID = currentFolderID ?? (searchActive || scope === 'all' ? undefined : 'none');
    const query = showFolders || scope === 'all' ? {
        scope,
        teamId: scope === 'channel' && channelId ? undefined : teamId,
        channelId: scope === 'channel' ? channelId : undefined,
        folderId: folderID,
        search: debouncedSearch,
        page,
        perPage: 100,
    } : null;
    const {data, loading, error} = useSavedLinks(query, revision + reload);
    const serverItems = data?.items ?? [];

    // Keep the order the user just made until the server confirms it. This keeps the list stable
    // while the reorder request and the follow-up refresh are in flight.
    const items = pendingItems ?? serverItems;

    useEffect(() => {
        if (!pendingItems || loading || !data) {
            return;
        }
        const serverIDs = data.items.map((savedLink) => savedLink.id);
        const pendingIDs = pendingItems.map((savedLink) => savedLink.id);
        const sameItems = serverIDs.length === pendingIDs.length && serverIDs.every((id) => pendingIDs.includes(id));
        if (!sameItems) {
            setPendingItems(null);
            return;
        }
        if (serverIDs.every((id, index) => id === pendingIDs[index])) {
            setPendingItems(null);
        }
    }, [data, loading, pendingItems]);

    const uniformGroup = items.length > 1 && items.every((savedLink) => savedLinkGroupKey(savedLink) === savedLinkGroupKey(items[0]));
    const canSort = !searchActive && scope !== 'all' && Boolean(data) && !data?.hasMore && page === 0 && uniformGroup;

    useEffect(() => {
        setPage(0);
    }, [scope, currentFolderID, debouncedSearch, teamId, channelId]);

    // A different page or scope shows a different list, so an optimistic order no longer applies.
    useEffect(() => {
        setPendingItems(null);
        queuedReorderRef.current = null;
    }, [page, scope, folderID, debouncedSearch, teamId, channelId]);

    useEffect(() => {
        if (data && data.items.length === 0 && data.total > 0 && page > 0) {
            setPage((value) => Math.max(0, value - 1));
        }
    }, [data, page]);

    const foldersById = new Map(folders.map((folder) => [folder.id, folder]));
    const foldersToShow = showFolders && !currentFolder && !searchActive ? folders : [];

    const scopeOptions = [
        {
            key: 'channel',
            label: (
                <FormattedMessage
                    id='savedLinks.scope.channel'
                    defaultMessage='Channel'
                />
            ),
        },
        {
            key: 'team',
            label: (
                <FormattedMessage
                    id='savedLinks.scope.team'
                    defaultMessage='Team'
                />
            ),
        },
        {
            key: 'all',
            label: (
                <FormattedMessage
                    id='savedLinks.scope.all'
                    defaultMessage='All'
                />
            ),
        },
    ];

    const labelFor = (savedLink: SavedLinkSummary) => {
        if (savedLink.channelId) {
            return channels[savedLink.channelId]?.display_name ?? savedLink.channelId;
        }
        return teams[savedLink.teamId]?.display_name ?? savedLink.teamId;
    };

    const createSavedLink = () => onCreate(scope, currentFolderID);

    const openFolder = (folder: FolderSummary) => {
        setCurrentFolder(folder);
        setPage(0);
        setSortError(undefined);
        setDraggedID(undefined);
        setDropTarget(undefined);
    };

    const closeFolder = () => {
        setCurrentFolder(undefined);
        setPage(0);
        setSortError(undefined);
        setDraggedID(undefined);
        setDropTarget(undefined);
    };

    const remove = async (savedLinkId: string) => {
        setDeleting(true);
        try {
            await deleteSavedLink(savedLinkId);
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

    const removeFolder = async () => {
        if (!folderToDelete) {
            return;
        }
        setDeletingFolder(true);
        try {
            await deleteFolder(folderToDelete.id);
            if (currentFolder?.id === folderToDelete.id) {
                setCurrentFolder(undefined);
            }
            setFolderToDelete(undefined);
            setReload((value) => value + 1);
            onDeleted();
        } catch (err) {
            setFolderToDelete(undefined);
            setFolderActionError(err instanceof Error ? err.message : String(err));
            setReload((value) => value + 1);
        } finally {
            setDeletingFolder(false);
        }
    };

    const folderSaved = (folder: Folder) => {
        const editing = Boolean(folderEditor?.folder);
        setFolderEditor(undefined);
        if (editing) {
            setCurrentFolder((current) => (current && current.id === folder.id ? {...current, name: folder.name} : current));
        } else {
            // Show the new folder at the root instead of staying inside the old one.
            setCurrentFolder(undefined);
        }
        setReload((value) => value + 1);
    };

    const move = async (newTeamID: string, newChannelID: string) => {
        if (moving?.folder) {
            await updateFolder(moving.folder.id, {teamId: newTeamID, channelId: newChannelID});
            setCurrentFolder(undefined);
        } else if (moving?.savedLink) {
            await updateSavedLink(moving.savedLink.id, {teamId: newTeamID, channelId: newChannelID});
        }
        setMoving(undefined);
        setReload((value) => value + 1);
        onDeleted();
    };

    const sendReorder = async (nextItems: SavedLinkSummary[]): Promise<void> => {
        queuedReorderRef.current = null;
        try {
            await reorderSavedLinks(nextItems.map((savedLink) => savedLink.id));
        } catch (err) {
            queuedReorderRef.current = null;
            setPendingItems(null);
            setSortError(err instanceof Error ? err.message : String(err));
            setReload((value) => value + 1);
            reorderingRef.current = false;
            return;
        }
        const queued = queuedReorderRef.current;
        if (queued) {
            await sendReorder(queued);
            return;
        }
        setReload((value) => value + 1);
        reorderingRef.current = false;
    };

    const flushReorder = (nextItems: SavedLinkSummary[]) => {
        setSortError(undefined);
        setDraggedID(undefined);
        setDropTarget(undefined);
        reorderingRef.current = true;
        sendReorder(nextItems);
    };

    // Show the new order immediately and queue it. Requests are sent one at a time so rapid moves
    // build on the optimistic order instead of racing each other.
    const submitReorder = (nextItems: SavedLinkSummary[]) => {
        if (nextItems.length === 0) {
            return;
        }
        setPendingItems(nextItems);
        queuedReorderRef.current = nextItems;
        if (!reorderingRef.current) {
            flushReorder(nextItems);
        }
    };

    const moveSavedLink = (savedLinkID: string, targetID: string, after: boolean) => {
        if (savedLinkID === targetID) {
            return;
        }
        const moving = items.find((savedLink) => savedLink.id === savedLinkID);
        if (!moving) {
            return;
        }
        const next = items.filter((savedLink) => savedLink.id !== savedLinkID);
        let targetIndex = next.findIndex((savedLink) => savedLink.id === targetID);
        if (targetIndex === -1) {
            return;
        }
        if (after) {
            targetIndex += 1;
        }
        next.splice(targetIndex, 0, moving);
        submitReorder(next);
    };

    const moveByOffset = (savedLinkID: string, offset: number) => {
        const from = items.findIndex((savedLink) => savedLink.id === savedLinkID);
        const to = from + offset;
        if (from < 0 || to < 0 || to >= items.length) {
            return;
        }
        const next = [...items];
        const [moving] = next.splice(from, 1);
        next.splice(to, 0, moving);
        submitReorder(next);
    };

    const handleDragStart = (event: DragEvent<HTMLButtonElement>, savedLinkID: string) => {
        if (!canSort) {
            event.preventDefault();
            return;
        }
        setDraggedID(savedLinkID);
        setDropTarget(undefined);
        event.dataTransfer.effectAllowed = 'move';
        event.dataTransfer.setData('text/plain', savedLinkID);
    };

    const handleDragOver = (event: DragEvent<HTMLLIElement>, savedLinkID: string) => {
        if (!canSort || !draggedID || draggedID === savedLinkID) {
            return;
        }
        event.preventDefault();
        event.dataTransfer.dropEffect = 'move';
        const rect = event.currentTarget.getBoundingClientRect();
        const after = event.clientY > rect.top + (rect.height / 2);
        setDropTarget((current) => (current?.id === savedLinkID && current.after === after ? current : {id: savedLinkID, after}));
    };

    const handleDrop = (event: DragEvent<HTMLLIElement>, savedLinkID: string) => {
        event.preventDefault();
        if (!canSort || !draggedID) {
            setDraggedID(undefined);
            setDropTarget(undefined);
            return;
        }
        const rect = event.currentTarget.getBoundingClientRect();
        const after = event.clientY > rect.top + (rect.height / 2);
        moveSavedLink(draggedID, savedLinkID, after);
        setDraggedID(undefined);
        setDropTarget(undefined);
    };

    const handleDragEnd = () => {
        setDraggedID(undefined);
        setDropTarget(undefined);
    };

    const handleReorderKeyDown = (event: KeyboardEvent<HTMLButtonElement>, savedLinkID: string) => {
        if (event.key === 'ArrowUp') {
            event.preventDefault();
            moveByOffset(savedLinkID, -1);
        } else if (event.key === 'ArrowDown') {
            event.preventDefault();
            moveByOffset(savedLinkID, 1);
        }
    };

    let emptyTitle: ReactNode;
    let emptyDescription: ReactNode;
    if (searchActive) {
        emptyTitle = (
            <FormattedMessage
                id='savedLinks.link.search_empty'
                defaultMessage='No saved links match your search'
            />
        );
        emptyDescription = (
            <FormattedMessage
                id='savedLinks.link.search_empty_description'
                defaultMessage='Try a different name, URL or tag.'
            />
        );
    } else if (currentFolder) {
        emptyTitle = (
            <FormattedMessage
                id='savedLinks.folder.empty'
                defaultMessage='This folder is empty'
            />
        );
        emptyDescription = (
            <FormattedMessage
                id='savedLinks.folder.empty_description'
                defaultMessage='Edit a saved link to move it into this folder.'
            />
        );
    } else {
        emptyTitle = (
            <FormattedMessage
                id='savedLinks.link.empty'
                defaultMessage='No saved links yet'
            />
        );
        emptyDescription = (
            <FormattedMessage
                id='savedLinks.link.empty_description'
                defaultMessage='Save a URL with the post menu or create one here.'
            />
        );
    }

    const movingTarget = moving?.folder ?? moving?.savedLink;
    const showEmpty = !loading && !error && data && items.length === 0 && foldersToShow.length === 0 && !foldersState.loading;

    // Keep the current rows mounted during a background refresh: swapping them for a spinner would
    // drop keyboard focus, scroll position and an in-progress drag.
    const showList = Boolean(data) && (foldersToShow.length > 0 || items.length > 0);

    return (
        <div className='SavedLinks-Panel'>
            <div className='SavedLinks-Panel__toolbar'>
                <Tabs
                    tabs={scopeOptions}
                    activeKey={scope}
                    onChange={(key) => setScope(key as SavedLinkScope)}
                    appearance='underlined'
                />
                <div className='SavedLinks-Panel__toolbarActions'>
                    {showFolders && (
                        <IconButton
                            icon={<Icon glyph={<FolderPlusOutlineIcon/>}/>}
                            aria-label={intl.formatMessage({id: 'savedLinks.folder.new', defaultMessage: 'New folder'})}
                            onClick={() => setFolderEditor({})}
                        />
                    )}
                    <IconButton
                        icon={<Icon glyph={<PlusIcon/>}/>}
                        aria-label={intl.formatMessage({id: 'savedLinks.link.new', defaultMessage: 'New saved link'})}
                        onClick={createSavedLink}
                    />
                </div>
            </div>
            <div className='SavedLinks-Panel__search'>
                <SearchInput
                    label={intl.formatMessage({id: 'savedLinks.link.search', defaultMessage: 'Search saved links'})}
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    onClear={() => setSearch('')}
                    clearLabel={intl.formatMessage({id: 'savedLinks.search.clear', defaultMessage: 'Clear search'})}
                />
            </div>

            {currentFolder && (
                <div className='SavedLinks-Panel__folderHeader'>
                    <IconButton
                        size='small'
                        icon={<Icon glyph={<ArrowLeftIcon/>}/>}
                        aria-label={intl.formatMessage({id: 'savedLinks.folder.back', defaultMessage: 'Back to all saved links'})}
                        onClick={closeFolder}
                    />
                    <div className='SavedLinks-Panel__folderHeaderTitle'>
                        <Icon
                            size='16'
                            glyph={<FolderOutlineIcon/>}
                        />
                        <span className='SavedLinks-Panel__folderName'>{currentFolder.name}</span>
                        <span className='SavedLinks-Panel__folderCount'>{currentFolder.savedLinkCount}</span>
                    </div>
                    <div className='SavedLinks-Panel__folderHeaderActions'>
                        <IconButton
                            size='small'
                            icon={<Icon glyph={<PencilOutlineIcon/>}/>}
                            aria-label={intl.formatMessage({id: 'savedLinks.folder.rename', defaultMessage: 'Rename folder'})}
                            onClick={() => setFolderEditor({folder: currentFolder})}
                        />
                        <IconButton
                            size='small'
                            icon={<Icon glyph={<FolderMoveOutlineIcon/>}/>}
                            aria-label={intl.formatMessage({id: 'savedLinks.folder.move', defaultMessage: 'Move folder'})}
                            onClick={() => setMoving({folder: currentFolder})}
                        />
                        <IconButton
                            size='small'
                            icon={<Icon glyph={<TrashCanOutlineIcon/>}/>}
                            aria-label={intl.formatMessage({id: 'savedLinks.folder.delete', defaultMessage: 'Delete folder'})}
                            onClick={() => setFolderToDelete(currentFolder)}
                        />
                    </div>
                </div>
            )}

            {actionError && (
                <SectionNotice
                    type='danger'
                    title={intl.formatMessage({id: 'savedLinks.link.delete_error', defaultMessage: 'Couldn’t delete the saved link'})}
                    description={actionError}
                    onDismiss={() => setActionError(undefined)}
                />
            )}
            {sortError && (
                <SectionNotice
                    type='danger'
                    title={intl.formatMessage({id: 'savedLinks.link.reorder_error', defaultMessage: 'Couldn’t reorder saved links'})}
                    description={sortError}
                    onDismiss={() => setSortError(undefined)}
                />
            )}
            {folderActionError && (
                <SectionNotice
                    type='danger'
                    title={intl.formatMessage({id: 'savedLinks.folder.delete_error', defaultMessage: 'Couldn’t delete the folder'})}
                    description={folderActionError}
                    onDismiss={() => setFolderActionError(undefined)}
                />
            )}
            {foldersState.error && (
                <SectionNotice
                    type='danger'
                    title={intl.formatMessage({id: 'savedLinks.folder.load_error', defaultMessage: 'Couldn’t load folders'})}
                    description={foldersState.error}
                    primaryButtonLabel={intl.formatMessage({id: 'savedLinks.retry', defaultMessage: 'Try again'})}
                    onPrimaryAction={() => setReload((value) => value + 1)}
                />
            )}
            {loading && !data && (
                <div className='SavedLinks-Panel__centered'>
                    <Spinner aria-label={intl.formatMessage({id: 'savedLinks.loading', defaultMessage: 'Loading…'})}/>
                </div>
            )}
            {error && !loading && (
                <SectionNotice
                    type='danger'
                    title={intl.formatMessage({id: 'savedLinks.link.load_error', defaultMessage: 'Couldn’t load saved links'})}
                    description={error}
                    primaryButtonLabel={intl.formatMessage({id: 'savedLinks.retry', defaultMessage: 'Try again'})}
                    onPrimaryAction={() => setReload((value) => value + 1)}
                />
            )}
            {showEmpty && (
                <EmptyState
                    title={emptyTitle}
                    description={emptyDescription}
                    action={{
                        children: intl.formatMessage({id: 'savedLinks.link.new', defaultMessage: 'New saved link'}),
                        emphasis: 'primary',
                        size: 'small',
                        onClick: createSavedLink,
                    }}
                />
            )}
            {showList && (
                <ul
                    className='SavedLinks-Panel__list'
                    aria-live='polite'
                >
                    {foldersToShow.map((folder) => (
                        <li
                            key={folder.id}
                            className='SavedLinks-Panel__folderItem'
                        >
                            <button
                                type='button'
                                className='SavedLinks-Panel__folderButton'
                                aria-label={intl.formatMessage({
                                    id: 'savedLinks.folder.open',
                                    defaultMessage: 'Open folder {name}, {count, plural, one {# saved link} other {# saved links}}',
                                }, {name: folder.name, count: folder.savedLinkCount})}
                                onClick={() => openFolder(folder)}
                            >
                                <Icon
                                    size='16'
                                    glyph={<FolderOutlineIcon/>}
                                />
                                <span className='SavedLinks-Panel__folderName'>{folder.name}</span>
                                <span className='SavedLinks-Panel__folderCount'>{folder.savedLinkCount}</span>
                                <Icon
                                    size='16'
                                    glyph={<ChevronRightIcon/>}
                                />
                            </button>
                        </li>
                    ))}
                    {items.map((savedLink) => {
                        const href = safeHttpUrl(savedLink.url);
                        const folder = !currentFolder && savedLink.folderId ? foldersById.get(savedLink.folderId) : undefined;
                        const dragging = draggedID === savedLink.id;
                        const drop = dropTarget?.id === savedLink.id ? dropTarget.after : undefined;
                        return (
                            <li
                                key={savedLink.id}
                                className={[
                                    'SavedLinks-Panel__item',
                                    dragging ? 'SavedLinks-Panel__item--dragging' : '',
                                    drop === false ? 'SavedLinks-Panel__item--dropBefore' : '',
                                    drop === true ? 'SavedLinks-Panel__item--dropAfter' : '',
                                ].filter(Boolean).join(' ')}
                                onDragOver={(event) => handleDragOver(event, savedLink.id)}
                                onDrop={(event) => handleDrop(event, savedLink.id)}
                            >
                                {canSort && (
                                    <button
                                        type='button'
                                        className='SavedLinks-Panel__dragHandle'
                                        draggable={true}
                                        aria-label={intl.formatMessage({
                                            id: 'savedLinks.link.reorder',
                                            defaultMessage: 'Drag to reorder. Use the arrow keys to move.',
                                        })}
                                        onDragStart={(event) => handleDragStart(event, savedLink.id)}
                                        onDragEnd={handleDragEnd}
                                        onKeyDown={(event) => handleReorderKeyDown(event, savedLink.id)}
                                    >
                                        <Icon glyph={<DragVerticalIcon/>}/>
                                    </button>
                                )}
                                <div className='SavedLinks-Panel__itemMain'>
                                    {href ? (
                                        // eslint-disable-next-line @mattermost/use-external-link -- plugin bundles cannot import host internals
                                        <a
                                            className='SavedLinks-Panel__title'
                                            href={href}
                                            target='_blank'
                                            rel='noreferrer noopener'
                                        >
                                            {savedLink.name || savedLink.url}
                                        </a>
                                    ) : (
                                        <span className='SavedLinks-Panel__title'>{savedLink.name || savedLink.url}</span>
                                    )}
                                    <span className='SavedLinks-Panel__url'>{savedLink.url}</span>
                                    <div className='SavedLinks-Panel__meta'>
                                        <span className='SavedLinks-Panel__scope'>{labelFor(savedLink)}</span>
                                        {folder && (
                                            <span className='SavedLinks-Panel__folder'>
                                                <Icon
                                                    size='10'
                                                    glyph={<FolderOutlineIcon/>}
                                                />
                                                {folder.name}
                                            </span>
                                        )}
                                        {savedLink.tags.map((tag) => (
                                            <Tag
                                                key={tag}
                                                label={tag}
                                                size='x-small'
                                            />
                                        ))}
                                    </div>
                                    {savedLink.sourcePostId && teams[savedLink.teamId] && (
                                        <span className='SavedLinks-Panel__source'>
                                            <a
                                                className='SavedLinks-Panel__sourceLink'
                                                href={`${getSitePath()}/${teams[savedLink.teamId].name}/pl/${savedLink.sourcePostId}`}
                                            >
                                                <FormattedMessage
                                                    id='savedLinks.link.open_post'
                                                    defaultMessage='Open the original post'
                                                />
                                            </a>
                                            {' · '}
                                            <FormattedMessage
                                                id='savedLinks.link.from_post'
                                                defaultMessage='Saved from a post'
                                            />
                                        </span>
                                    )}
                                </div>
                                <div className='SavedLinks-Panel__actions'>
                                    {confirmDelete === savedLink.id ? (
                                        <>
                                            <Button
                                                emphasis='tertiary'
                                                size='small'
                                                onClick={() => setConfirmDelete(undefined)}
                                                disabled={deleting}
                                            >
                                                <FormattedMessage
                                                    id='savedLinks.cancel'
                                                    defaultMessage='Cancel'
                                                />
                                            </Button>
                                            <Button
                                                emphasis='primary'
                                                size='small'
                                                destructive={true}
                                                loading={deleting}
                                                onClick={() => remove(savedLink.id)}
                                            >
                                                <FormattedMessage
                                                    id='savedLinks.link.delete_confirm'
                                                    defaultMessage='Delete'
                                                />
                                            </Button>
                                        </>
                                    ) : (
                                        <>
                                            <IconButton
                                                size='small'
                                                icon={<Icon glyph={<PencilOutlineIcon/>}/>}
                                                aria-label={intl.formatMessage({id: 'savedLinks.link.edit', defaultMessage: 'Edit saved link'})}
                                                onClick={() => onEdit(savedLink.id)}
                                            />
                                            {!savedLink.sourcePostId && (
                                                <IconButton
                                                    size='small'
                                                    icon={<Icon glyph={<FolderMoveOutlineIcon/>}/>}
                                                    aria-label={intl.formatMessage({id: 'savedLinks.link.move', defaultMessage: 'Move saved link'})}
                                                    onClick={() => setMoving({savedLink})}
                                                />
                                            )}
                                            <IconButton
                                                size='small'
                                                icon={<Icon glyph={<TrashCanOutlineIcon/>}/>}
                                                aria-label={intl.formatMessage({id: 'savedLinks.link.delete', defaultMessage: 'Delete saved link'})}
                                                onClick={() => setConfirmDelete(savedLink.id)}
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
                <div className='SavedLinks-Panel__pagination'>
                    <Button
                        emphasis='tertiary'
                        size='small'
                        disabled={page === 0}
                        onClick={() => setPage((value) => Math.max(0, value - 1))}
                    >
                        <FormattedMessage
                            id='savedLinks.pagination.previous'
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
                            id='savedLinks.pagination.next'
                            defaultMessage='Next'
                        />
                    </Button>
                </div>
            )}

            {folderEditor && (
                <FolderEditor
                    folder={folderEditor.folder}
                    teamId={teamId}
                    channelId={folderChannelId}
                    onClose={() => setFolderEditor(undefined)}
                    onSaved={folderSaved}
                />
            )}
            {movingTarget && (
                <MoveDialog
                    title={moving?.folder ? (
                        <FormattedMessage
                            id='savedLinks.folder.move_title'
                            defaultMessage='Move folder'
                        />
                    ) : (
                        <FormattedMessage
                            id='savedLinks.link.move_title'
                            defaultMessage='Move saved link'
                        />
                    )}
                    teamId={movingTarget.teamId}
                    channelId={movingTarget.channelId}
                    hint={moving?.folder ? (
                        <FormattedMessage
                            id='savedLinks.folder.move_hint'
                            defaultMessage='The folder and the saved links in it move together.'
                        />
                    ) : (
                        <FormattedMessage
                            id='savedLinks.link.move_hint'
                            defaultMessage='The saved link leaves its folder when it moves.'
                        />
                    )}
                    onMove={move}
                    onClose={() => setMoving(undefined)}
                />
            )}
            {folderToDelete && (
                <Dialog
                    title={(
                        <FormattedMessage
                            id='savedLinks.folder.delete_title'
                            defaultMessage='Delete folder'
                        />
                    )}
                    onClose={() => setFolderToDelete(undefined)}
                    footer={(
                        <>
                            <Button
                                emphasis='tertiary'
                                onClick={() => setFolderToDelete(undefined)}
                                disabled={deletingFolder}
                            >
                                <FormattedMessage
                                    id='savedLinks.cancel'
                                    defaultMessage='Cancel'
                                />
                            </Button>
                            <Button
                                emphasis='primary'
                                destructive={true}
                                loading={deletingFolder}
                                onClick={removeFolder}
                            >
                                <FormattedMessage
                                    id='savedLinks.folder.delete_confirm'
                                    defaultMessage='Delete folder'
                                />
                            </Button>
                        </>
                    )}
                >
                    <p className='SavedLinks-Panel__dialogText'>
                        <FormattedMessage
                            id='savedLinks.folder.delete_message'
                            defaultMessage='Saved links in “{name}” will become ungrouped. The links themselves are not deleted.'
                            values={{name: folderToDelete.name}}
                        />
                    </p>
                </Dialog>
            )}
        </div>
    );
}
