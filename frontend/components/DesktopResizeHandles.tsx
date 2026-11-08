"use client";

import { MouseEvent } from "react";

export type DesktopResizeEdge =
	| "n-resize"
	| "ne-resize"
	| "e-resize"
	| "se-resize"
	| "s-resize"
	| "sw-resize"
	| "w-resize"
	| "nw-resize";

type DesktopResizeHandlesProps = {
	enabled: boolean;
	onResizeStart: (edge: DesktopResizeEdge) => void;
};

const edgeHandles: Array<{ edge: DesktopResizeEdge; className: string }> = [
	{ edge: "n-resize", className: "inset-x-0 top-0 h-2 cursor-ns-resize" },
	{ edge: "e-resize", className: "inset-y-0 right-0 w-2 cursor-ew-resize" },
	{ edge: "s-resize", className: "inset-x-0 bottom-0 h-2 cursor-ns-resize" },
	{ edge: "w-resize", className: "inset-y-0 left-0 w-2 cursor-ew-resize" },
];

const cornerHandles: Array<{ edge: DesktopResizeEdge; className: string }> = [
	{ edge: "nw-resize", className: "left-0 top-0 h-3 w-3 cursor-nwse-resize" },
	{ edge: "ne-resize", className: "right-0 top-0 h-3 w-3 cursor-nesw-resize" },
	{ edge: "se-resize", className: "bottom-0 right-0 h-3 w-3 cursor-nwse-resize" },
	{ edge: "sw-resize", className: "bottom-0 left-0 h-3 w-3 cursor-nesw-resize" },
];

export function DesktopResizeHandles({ enabled, onResizeStart }: DesktopResizeHandlesProps) {
	if (!enabled) {
		return null;
	}

	const handleMouseDown = (event: MouseEvent<HTMLDivElement>, edge: DesktopResizeEdge) => {
		if (event.button !== 0) {
			return;
		}
		event.preventDefault();
		event.stopPropagation();
		onResizeStart(edge);
	};

	return (
		<div aria-hidden="true" className="pointer-events-none fixed inset-0 z-[1000]">
			{edgeHandles.map(({ edge, className }) => (
				<div
					key={edge}
					className={`pointer-events-auto fixed ${className}`}
					onMouseDown={(event) => handleMouseDown(event, edge)}
				/>
			))}
			{cornerHandles.map(({ edge, className }) => (
				<div
					key={edge}
					className={`pointer-events-auto fixed z-10 ${className}`}
					onMouseDown={(event) => handleMouseDown(event, edge)}
				/>
			))}
		</div>
	);
}
