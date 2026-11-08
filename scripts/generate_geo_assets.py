#!/usr/bin/env python3
"""Regenerate the offline map assets used by the asset map page.

Sources:
  * China province boundaries: DataV.GeoAtlas (geo.datav.aliyun.com)
  * World country boundaries: johan/world.geo.json (public domain)
  * City coordinates: centroids from the DataV city-level responses

The frontend never calls those endpoints at runtime; the generated files are
served from frontend/public/geo and frontend/lib/asset-map.

Usage:
  python3 scripts/generate_geo_assets.py
"""

import json
import math
import tempfile
import urllib.request
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
CACHE_DIR = Path(tempfile.gettempdir()) / "hephaestus-geo-cache"
CHINA_URL = "https://geo.datav.aliyun.com/areas_v3/bound/100000_full.json"
WORLD_URL = "https://cdn.jsdelivr.net/gh/johan/world.geo.json@master/countries.geo.json"
AREA_URL = "https://geo.datav.aliyun.com/areas_v3/bound/{adcode}_full.json"


def fetch(url, cache_name):
    CACHE_DIR.mkdir(parents=True, exist_ok=True)
    path = CACHE_DIR / cache_name
    if not path.exists():
        with urllib.request.urlopen(url, timeout=60) as response:
            path.write_bytes(response.read())
    return json.loads(path.read_text())


def rdp(points, eps):
    if len(points) < 3:
        return points
    keep = [False] * len(points)
    keep[0] = keep[-1] = True
    stack = [(0, len(points) - 1)]
    while stack:
        start, end = stack.pop()
        if end <= start + 1:
            continue
        ax, ay = points[start]
        bx, by = points[end]
        dx, dy = bx - ax, by - ay
        norm = math.hypot(dx, dy)
        best = -1.0
        index = -1
        for i in range(start + 1, end):
            px, py = points[i]
            if norm == 0:
                dist = math.hypot(px - ax, py - ay)
            else:
                dist = abs(dy * px - dx * py + bx * ay - by * ax) / norm
            if dist > best:
                best = dist
                index = i
        if best > eps:
            keep[index] = True
            stack.append((start, index))
            stack.append((index, end))
    return [point for point, kept in zip(points, keep) if kept]


def simplify_ring(ring, eps):
    points = [(round(lng, 4), round(lat, 4)) for lng, lat in ring]
    points = rdp(points, eps)
    if len(points) < 4:
        return None
    if points[0] != points[-1]:
        points.append(points[0])
    return [[lng, lat] for lng, lat in points]


def simplify_geometry(geom, eps):
    if geom["type"] == "Polygon":
        rings = [simplify_ring(ring, eps) for ring in geom["coordinates"]]
        rings = [ring for ring in rings if ring]
        if not rings:
            return None
        return {"type": "Polygon", "coordinates": rings}
    if geom["type"] == "MultiPolygon":
        polygons = []
        for polygon in geom["coordinates"]:
            rings = [simplify_ring(ring, eps) for ring in polygon]
            rings = [ring for ring in rings if ring]
            if rings:
                polygons.append(rings)
        if not polygons:
            return None
        return {"type": "MultiPolygon", "coordinates": polygons}
    return geom


def build_china():
    data = fetch(CHINA_URL, "china.json")
    features = []
    for feature in data["features"]:
        props = feature["properties"]
        geometry = simplify_geometry(feature["geometry"], 0.012)
        if geometry is None:
            continue
        center = props.get("center") or props.get("centroid") or [0, 0]
        features.append({
            "type": "Feature",
            "properties": {
                "name": props.get("name") or "南海诸岛",
                "adcode": props.get("adcode") or 0,
                "center": [round(center[0], 4), round(center[1], 4)],
            },
            "geometry": geometry,
        })
    return {"type": "FeatureCollection", "features": features}


def build_world():
    data = fetch(WORLD_URL, "world.json")
    features = []
    for feature in data["features"]:
        geometry = simplify_geometry(feature["geometry"], 0.03)
        if geometry is None:
            continue
        features.append({
            "type": "Feature",
            "properties": {"name": feature["properties"].get("name", "")},
            "geometry": geometry,
        })
    return {"type": "FeatureCollection", "features": features}


def build_city_coordinates(china):
    cities = {}
    municipalities = {"北京市", "天津市", "上海市", "重庆市", "香港特别行政区", "澳门特别行政区"}
    for feature in china["features"]:
        props = feature["properties"]
        if props.get("name") in municipalities:
            key = props["name"].replace("市", "").replace("特别行政区", "")
            cities[key] = props["center"]
    for feature in china["features"]:
        props = feature["properties"]
        adcode = props.get("adcode")
        if not adcode:
            continue
        try:
            area = fetch(AREA_URL.format(adcode=adcode), f"area-{adcode}.json")
        except Exception:
            continue
        for child in area.get("features", []):
            child_props = child.get("properties", {})
            if child_props.get("level") not in ("city", "district"):
                continue
            name = (child_props.get("name") or "").strip()
            center = child_props.get("center") or child_props.get("centroid")
            if not name or not center:
                continue
            key = name.removesuffix("市").removesuffix("地区").removesuffix("自治州").removesuffix("特别行政区")
            if not key or key in cities:
                continue
            cities[key] = [round(center[0], 4), round(center[1], 4)]
    return cities


def main():
    china = build_china()
    world = build_world()
    cities = build_city_coordinates(china)

    public_dir = REPO_ROOT / "frontend" / "public" / "geo"
    public_dir.mkdir(parents=True, exist_ok=True)
    public_dir.joinpath("china-provinces.json").write_text(
        json.dumps(china, ensure_ascii=False, separators=(",", ":"))
    )
    public_dir.joinpath("world-countries.json").write_text(
        json.dumps(world, ensure_ascii=False, separators=(",", ":"))
    )

    lib_dir = REPO_ROOT / "frontend" / "lib" / "asset-map"
    lib_dir.mkdir(parents=True, exist_ok=True)
    lines = [
        "// Generated by scripts/generate_geo_assets.py from DataV area data. Do not edit by hand.",
        "",
        "export type CityCoordinate = readonly [number, number]",
        "",
        "export const CITY_COORDINATES: Record<string, CityCoordinate> = {",
    ]
    for name in sorted(cities):
        lng, lat = cities[name]
        safe = name.replace("\\", "\\\\").replace('"', '\\"')
        lines.append(f'  "{safe}": [{lng}, {lat}],')
    lines.append("}")
    lib_dir.joinpath("city-coordinates.ts").write_text("\n".join(lines) + "\n")

    print(f"china provinces: {len(china['features'])}")
    print(f"world countries: {len(world['features'])}")
    print(f"city coordinates: {len(cities)}")


if __name__ == "__main__":
    main()
