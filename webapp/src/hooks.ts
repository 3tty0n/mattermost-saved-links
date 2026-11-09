// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {fetchBookmarks} from 'client';
import {useEffect, useRef, useState} from 'react';

import type {BookmarkQuery, BookmarkSummary, ListResult} from 'types';

type AsyncState<T> = {
    data?: T;
    loading: boolean;
    error?: string;
};

function errorMessage(error: unknown): string {
    if (error instanceof Error) {
        return error.message;
    }
    return String(error);
}

export function useBookmarks(query: BookmarkQuery | null, revision: number) {
    const queryKey = query ? JSON.stringify(query) : '';
    const [state, setState] = useState<AsyncState<ListResult<BookmarkSummary>>>({loading: Boolean(query)});
    const previousKey = useRef('');

    useEffect(() => {
        const keyChanged = previousKey.current !== queryKey;
        previousKey.current = queryKey;
        if (!queryKey) {
            setState({loading: false});
            return undefined;
        }
        const controller = new AbortController();
        setState((previous) => (keyChanged ? {loading: true} : {...previous, loading: true, error: undefined}));
        fetchBookmarks(JSON.parse(queryKey) as BookmarkQuery, controller.signal).then(
            (data) => setState({data, loading: false}),
            (error: unknown) => {
                if (controller.signal.aborted) {
                    return;
                }
                setState({loading: false, error: errorMessage(error)});
            },
        );
        return () => controller.abort();
    }, [queryKey, revision]);

    return state;
}

export function useDebouncedValue<T>(value: T, delay: number): T {
    const [debounced, setDebounced] = useState(value);
    useEffect(() => {
        const timer = window.setTimeout(() => setDebounced(value), delay);
        return () => window.clearTimeout(timer);
    }, [value, delay]);
    return debounced;
}
