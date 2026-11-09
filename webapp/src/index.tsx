// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

// Compass tokens are declared in cascade layers, so this is safe to load next to the host's
// copy. Never import '@mattermost/compass-ui/styles/standalone' here: it overrides the theme.
import '@mattermost/compass-ui/styles';

import {setSiteURL} from 'client';
import manifest from 'manifest';
import {FormattedMessage} from 'react-intl';
import type {Store} from 'redux';
import {openComposer, reconnected, reducer, remoteEvent} from 'state';
import {extractUrls, postTitle} from 'utils';

import LinkVariantIcon from '@mattermost/compass-icons/components/link-variant';
import type {Channel} from '@mattermost/types/channels';
import type {Post} from '@mattermost/types/posts';
import type {GlobalState} from '@mattermost/types/store';

import {selectSiteURL} from 'selectors';

import BookmarksSidebar from 'components/bookmarks_sidebar/bookmarks_sidebar';

import type {ComposerRequest} from 'types';
import type {PluginRegistry} from 'types/mattermost-webapp';

import en from '../i18n/en.json';

const translations: Record<string, Record<string, string>> = {en};

function usablePost(state: GlobalState, postID: string): {post: Post; channel: Channel} | undefined {
    if (!postID) {
        return undefined;
    }
    const post = state.entities.posts.posts[postID];
    if (!post || post.delete_at) {
        return undefined;
    }
    const channel = state.entities.channels.channels[post.channel_id];
    if (!channel || (channel.type !== 'O' && channel.type !== 'P')) {
        return undefined;
    }
    return {post, channel};
}

export default class Plugin {
    public async initialize(registry: PluginRegistry, store: Store<GlobalState>) {
        setSiteURL(selectSiteURL(store.getState()));

        registry.registerTranslations((locale) => translations[locale] ?? {});
        registry.registerReducer(reducer);
        registry.registerWebSocketEventHandler(`custom_${manifest.id}_changed`, () => store.dispatch(remoteEvent()));
        registry.registerReconnectHandler(() => store.dispatch(reconnected()));

        const title = (
            <FormattedMessage
                id='bookmarks.title'
                defaultMessage='Bookmarks'
            />
        );
        const rhs = registry.registerRightHandSidebarComponent(BookmarksSidebar, title);

        registry.registerChannelHeaderButtonAction(
            <LinkVariantIcon size={18}/>,
            () => store.dispatch(rhs.toggleRHSPlugin),
            title,
            title,
        );

        const saveBookmarkFromPost = (postID: string) => {
            const target = usablePost(store.getState(), postID);
            if (!target) {
                return;
            }
            const urls = extractUrls(target.post.message);
            if (urls.length === 0) {
                return;
            }
            const request: ComposerRequest = {
                kind: 'bookmark',
                postId: postID,
                channelId: target.post.channel_id,
                title: postTitle(target.post.message),
                description: target.post.message,
                urls,
            };
            store.dispatch(openComposer(request));
            store.dispatch(rhs.showRHSPlugin);
        };

        registry.registerPostDropdownMenuAction(
            <FormattedMessage
                id='bookmarks.post.save'
                defaultMessage='Save URL as bookmark'
            />,
            saveBookmarkFromPost,
            (postID: string) => {
                const target = usablePost(store.getState(), postID);
                return Boolean(target && extractUrls(target.post.message).length > 0);
            },
        );
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
