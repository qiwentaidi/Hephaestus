#import "screenshot_native_darwin.h"

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

#include <errno.h>
#include <math.h>
#include <poll.h>
#include <stdatomic.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

struct HephaestusSnapshot {
	atomic_int references;
	int read_fd;
	int write_fd;
	void *bytes;
	size_t length;
	int logical_width;
	int logical_height;
};

static WKWebView *hephaestusFindWebView(NSView *view) {
	if (view == nil) {
		return nil;
	}
	if ([view isKindOfClass:[WKWebView class]]) {
		return (WKWebView *)view;
	}
	for (NSView *subview in [view subviews]) {
		WKWebView *found = hephaestusFindWebView(subview);
		if (found != nil) {
			return found;
		}
	}
	return nil;
}

static void hephaestusSnapshotRetain(HephaestusSnapshot *snapshot) {
	if (snapshot != NULL) {
		atomic_fetch_add(&snapshot->references, 1);
	}
}

static void hephaestusSnapshotSignal(HephaestusSnapshot *snapshot) {
	if (snapshot == NULL) {
		return;
	}
	char byte = 1;
	ssize_t ignored = write(snapshot->write_fd, &byte, 1);
	(void)ignored;
}

HephaestusSnapshot *hephaestusSnapshotCreate(void) {
	HephaestusSnapshot *snapshot = calloc(1, sizeof(HephaestusSnapshot));
	if (snapshot == NULL) {
		return NULL;
	}
	int descriptors[2] = {-1, -1};
	if (pipe(descriptors) != 0) {
		free(snapshot);
		return NULL;
	}
	snapshot->read_fd = descriptors[0];
	snapshot->write_fd = descriptors[1];
	atomic_init(&snapshot->references, 1);
	return snapshot;
}

void hephaestusSnapshotStart(void *nsWindow, HephaestusSnapshot *snapshot) {
	if (snapshot == NULL) {
		return;
	}
	hephaestusSnapshotRetain(snapshot);

	NSWindow *window = (__bridge NSWindow *)nsWindow;
	WKWebView *webView = window != nil ? hephaestusFindWebView(window.contentView) : nil;
	if (webView == nil) {
		hephaestusSnapshotSignal(snapshot);
		hephaestusSnapshotRelease(snapshot);
		return;
	}

	WKSnapshotConfiguration *configuration = [[WKSnapshotConfiguration alloc] init];
	configuration.rect = webView.bounds;
	configuration.afterScreenUpdates = YES;

	[webView takeSnapshotWithConfiguration:configuration
	                     completionHandler:^(NSImage *image, NSError *error) {
		@autoreleasepool {
			if (image != nil) {
				snapshot->logical_width = (int)llround(image.size.width);
				snapshot->logical_height = (int)llround(image.size.height);
				CGImageRef cgImage = [image CGImageForProposedRect:NULL context:nil hints:nil];
				if (cgImage != NULL) {
					NSBitmapImageRep *representation = [[NSBitmapImageRep alloc] initWithCGImage:cgImage];
					NSData *png = [representation representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
					if (png.length > 0) {
						void *copy = malloc(png.length);
						if (copy != NULL) {
							memcpy(copy, png.bytes, png.length);
							snapshot->bytes = copy;
							snapshot->length = png.length;
						}
					}
				}
			}
			hephaestusSnapshotSignal(snapshot);
			hephaestusSnapshotRelease(snapshot);
		}
	}];
}

int hephaestusSnapshotWait(HephaestusSnapshot *snapshot, int timeoutMillis) {
	if (snapshot == NULL) {
		return 0;
	}
	struct pollfd descriptor = {
		.fd = snapshot->read_fd,
		.events = POLLIN,
	};
	int result;
	do {
		result = poll(&descriptor, 1, timeoutMillis);
	} while (result < 0 && errno == EINTR);
	if (result <= 0) {
		return 0;
	}
	char byte;
	ssize_t ignored;
	do {
		ignored = read(snapshot->read_fd, &byte, 1);
	} while (ignored < 0 && errno == EINTR);
	return 1;
}

const void *hephaestusSnapshotData(HephaestusSnapshot *snapshot) {
	return snapshot != NULL ? snapshot->bytes : NULL;
}

size_t hephaestusSnapshotLength(HephaestusSnapshot *snapshot) {
	return snapshot != NULL ? snapshot->length : 0;
}

int hephaestusSnapshotWidth(HephaestusSnapshot *snapshot) {
	return snapshot != NULL ? snapshot->logical_width : 0;
}

int hephaestusSnapshotHeight(HephaestusSnapshot *snapshot) {
	return snapshot != NULL ? snapshot->logical_height : 0;
}

void hephaestusSnapshotRelease(HephaestusSnapshot *snapshot) {
	if (snapshot == NULL) {
		return;
	}
	if (atomic_fetch_sub(&snapshot->references, 1) != 1) {
		return;
	}
	if (snapshot->bytes != NULL) {
		free(snapshot->bytes);
	}
	if (snapshot->read_fd >= 0) {
		close(snapshot->read_fd);
	}
	if (snapshot->write_fd >= 0) {
		close(snapshot->write_fd);
	}
	free(snapshot);
}
