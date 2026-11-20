#ifndef HEPHAESTUS_SCREENSHOT_NATIVE_DARWIN_H
#define HEPHAESTUS_SCREENSHOT_NATIVE_DARWIN_H

#include <stddef.h>

typedef struct HephaestusSnapshot HephaestusSnapshot;

HephaestusSnapshot *hephaestusSnapshotCreate(void);
void hephaestusSnapshotStart(void *nsWindow, HephaestusSnapshot *snapshot);
int hephaestusSnapshotWait(HephaestusSnapshot *snapshot, int timeoutMillis);
const void *hephaestusSnapshotData(HephaestusSnapshot *snapshot);
size_t hephaestusSnapshotLength(HephaestusSnapshot *snapshot);
int hephaestusSnapshotWidth(HephaestusSnapshot *snapshot);
int hephaestusSnapshotHeight(HephaestusSnapshot *snapshot);
void hephaestusSnapshotRelease(HephaestusSnapshot *snapshot);

#endif
