// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { Block } from "@/app/block/block";
import type { ContentRenderer, NodeModel, PreviewRenderer } from "@/layout/index";
import type { TileLayoutContents } from "@/layout/lib/types";
import * as services from "@/store/services";

// As TabContent (app/tab/tabcontent.tsx): closing a pane deletes its block. The server keeps a builder
// tab when its last block goes, so the panel can show its empty state.
export function makeBuilderTileContents(tabId: string, gapSizePx: number): TileLayoutContents {
    const renderContent: ContentRenderer = (nodeModel: NodeModel) => {
        return <Block key={nodeModel.blockId} nodeModel={nodeModel} preview={false} />;
    };
    const renderPreview: PreviewRenderer = (nodeModel: NodeModel) => {
        return <Block key={nodeModel.blockId} nodeModel={nodeModel} preview={true} />;
    };
    function onNodeDelete(data: TabLayoutData) {
        return services.ObjectService.DeleteBlock(data.blockId);
    }
    return { renderContent, renderPreview, tabId, onNodeDelete, gapSizePx };
}
