// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

const URL_PATTERN = /https?:\/\/[^\s<>"']+/gi;
const URL_ONLY_PATTERN = /^https?:\/\/\S+$/i;

// extractUrls finds http(s) links in a post. Trailing punctuation is stripped because users
// often end a sentence immediately after a URL.
export function extractUrls(message: string): string[] {
    const found = message.match(URL_PATTERN) ?? [];
    const unique: string[] = [];
    for (const raw of found) {
        const url = raw.replace(/[.,;:!?)\]}]+$/, '');
        if (url && !unique.includes(url)) {
            unique.push(url);
        }
    }
    return unique;
}

// postName turns a post's first line into a sensible saved link name.
export function postName(message: string): string {
    const line = message.split('\n').find((candidate) => candidate.trim() !== '') ?? '';
    const cleaned = line.
        replace(/^[\s>#*+-]+/, '').
        replace(/[*_`~]/g, '').
        trim();
    if (cleaned === '' || URL_ONLY_PATTERN.test(cleaned)) {
        return '';
    }
    return cleaned.length > 200 ? `${cleaned.slice(0, 199)}…` : cleaned;
}
