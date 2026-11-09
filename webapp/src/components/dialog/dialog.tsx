// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import type {ReactNode} from 'react';

import {Modal} from '@mattermost/compass-ui/components/modal';

import './dialog.css';

type Props = {
    title: ReactNode;
    onClose: () => void;
    footer?: ReactNode;
    children: ReactNode;
};

// Dialog owns the backdrop and stacking; Compass Modal owns the dialog chrome, focus trap and
// Escape handling.
export default function Dialog({title, onClose, footer, children}: Props) {
    return (
        <div
            className='SavedLinks-DialogOverlay'
            onMouseDown={(event) => {
                if (event.target === event.currentTarget) {
                    onClose();
                }
            }}
            role='presentation'
        >
            <Modal
                className='SavedLinks-Dialog'
                title={title}
                onClose={onClose}
                footer={footer}
                open={true}
                size='small'
            >
                {children}
            </Modal>
        </div>
    );
}
