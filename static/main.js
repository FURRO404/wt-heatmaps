const svg = document.getElementById("mapview");

let vb = {
	x: 0,
	y: 0,
	w: 2048,
	h: 2048,
};
setViewBox();

svg.addEventListener("wheel", (e) => {
	e.preventDefault();
	const factor = 1.1;
	const zoom = e.deltaY < 0 ? 1 / factor : factor;

	const rect = svg.getBoundingClientRect();
	const mx = e.clientX - rect.left;
	const my = e.clientY - rect.top;

	const sx = vb.x + (mx / rect.width) * vb.w;
	const sy = vb.y + (my / rect.height) * vb.h;

	vb.w *= zoom;
	vb.h *= zoom;

	vb.x = sx - (mx / rect.width) * vb.w;
	vb.y = sy - (my / rect.height) * vb.h;

	setViewBox();
});

svg.addEventListener("pointermove", (e) => {
	if (e.buttons == 0) {
		return;
	}
	if (selectFrom != null) {
		drawSelection(svgPoint(e));
		return;
	}
	const dx = (e.movementX / svg.clientWidth) * vb.w;
	const dy = (e.movementY / svg.clientHeight) * vb.h;

	vb.x -= dx;
	vb.y -= dy;

	setViewBox();
});

svg.addEventListener("dblclick", () => {
	vb = { x: 0, y: 0, w: 2048, h: 2048 };
	setViewBox();
});

function setViewBox() {
	svg.setAttribute("viewBox", `${vb.x} ${vb.y} ${vb.w} ${vb.h}`);
}

document.getElementById("tankmapBrightnessSlider").oninput = (e) => {
	for (let v of document.styleSheets) {
		for (let v2 of v.cssRules) {
			if (v2.selectorText == "#tankmap") {
				v2.style.filter = `brightness(${e.target.value}%)`;
			}
		}
	}
};

const form = document.querySelector("#settingsForm");

form.addEventListener("submit", (e) => {
	if (e.submitter.id != "settingsSubmitBtn") {
		return;
	}
	e.preventDefault();
	let f = new FormData(form);
	if (f.get("level") == "") {
		return;
	}
	// the table on the page counted the filters as they were, so it goes
	setAreaLoaded(false);
	loadHeat(f.get("level"), "/heat?" + new URLSearchParams(f).toString());
	document
		.getElementById("tankmap")
		.setAttribute("href", "/minimap/2048/" + f.get("level"));
});

// The heatmap is one pixel per world meter, so its own pixel size is the grid
// an area selection snaps to. An svg image gives no intrinsic size, so the
// bytes come through fetch and go into the image as a blob. That is one
// request, and the size is known by the time the map is on the screen.
const heatImage = document.getElementById("heat");
const heatLoading = document.getElementById("heatLoading");
// heat is the heatmap on the screen, or null while there is none. A load that
// fails leaves the one before it up, so this always tells what the user sees.
let heat = null;
let heatLoad = 0;

async function loadHeat(level, url) {
	const load = ++heatLoad;
	heatLoading.classList.add("loading");
	const probe = new Image();
	try {
		const resp = await fetch(url);
		if (!resp.ok) {
			return;
		}
		probe.src = URL.createObjectURL(await resp.blob());
		// a 204 answer carries no image, so this rejects and the map stays
		await probe.decode();
	} catch {
		if (probe.src != "") {
			URL.revokeObjectURL(probe.src);
		}
		return;
	} finally {
		// the note belongs to the newest load, so an overtaken one leaves it up
		if (load == heatLoad) {
			heatLoading.classList.remove("loading");
		}
	}
	if (load != heatLoad) {
		URL.revokeObjectURL(probe.src);
		return;
	}
	if (heat != null) {
		URL.revokeObjectURL(heat.url);
	}
	heat = { level, w: probe.naturalWidth, h: probe.naturalHeight, url: probe.src };
	heatImage.setAttribute("href", heat.url);
}

// area selection, drags a box and asks the server for the vehicles in it

const mapSize = 2048;
const selectBtn = document.getElementById("areaSelectBtn");
const selectRect = document.getElementById("areaSelect");
const areaResults = document.getElementById("areaStatsResults");
let selectMode = false;
let selectFrom = null;
let areaLoaded = false;

selectBtn.addEventListener("click", () => {
	if (areaLoaded) {
		setAreaLoaded(false);
		return;
	}
	setSelectMode(!selectMode);
});

function setSelectMode(on) {
	selectMode = on;
	svg.style.cursor = on ? "crosshair" : "";
	selectBtn.style.fontWeight = on ? "bold" : "";
}

function setAreaLoaded(on) {
	areaLoaded = on;
	selectBtn.textContent = on ? "Clear area" : "Select area";
	if (!on) {
		selectRect.style.display = "none";
		areaResults.innerHTML = "";
	}
}

// snapToGrid puts a map coordinate on the nearest edge between two heatmap
// pixels, and holds it inside the map. cells is the pixel count of the heatmap
// on that axis, one pixel being one world meter.
function snapToGrid(v, cells) {
	const cell = Math.min(Math.max(Math.round((v / mapSize) * cells), 0), cells);
	return (cell / cells) * mapSize;
}

function svgPoint(e) {
	const rect = svg.getBoundingClientRect();
	const p = {
		x: vb.x + ((e.clientX - rect.left) / rect.width) * vb.w,
		y: vb.y + ((e.clientY - rect.top) / rect.height) * vb.h,
	};
	// a map with no heatmap on it yet has no grid to snap to
	if (heat != null) {
		p.x = snapToGrid(p.x, heat.w);
		p.y = snapToGrid(p.y, heat.h);
	}
	return p;
}

function drawSelection(to) {
	selectRect.setAttribute("x", Math.min(selectFrom.x, to.x));
	selectRect.setAttribute("y", Math.min(selectFrom.y, to.y));
	selectRect.setAttribute("width", Math.abs(to.x - selectFrom.x));
	selectRect.setAttribute("height", Math.abs(to.y - selectFrom.y));
	selectRect.style.display = "";
}

svg.addEventListener("pointerdown", (e) => {
	if (!selectMode) {
		return;
	}
	e.preventDefault();
	svg.setPointerCapture(e.pointerId);
	selectFrom = svgPoint(e);
	drawSelection(selectFrom);
});

svg.addEventListener("pointercancel", () => {
	selectFrom = null;
	selectRect.style.display = "none";
	setSelectMode(false);
});

svg.addEventListener("pointerup", (e) => {
	if (selectFrom == null) {
		return;
	}
	svg.releasePointerCapture(e.pointerId);
	const from = selectFrom;
	const to = svgPoint(e);
	selectFrom = null;
	setSelectMode(false);
	if (heat == null || from.x == to.x || from.y == to.y) {
		selectRect.style.display = "none";
		return;
	}
	const p = new URLSearchParams(new FormData(form));
	// the box was drawn on the heatmap, so it belongs to the level the heatmap
	// was drawn for, whatever the form says now. The other filters do come from
	// the form, which is what the page promises.
	p.set("level", heat.level);
	p.set("u0", from.x / mapSize);
	p.set("v0", from.y / mapSize);
	p.set("u1", to.x / mapSize);
	p.set("v1", to.y / mapSize);
	// htmx swaps nothing when the server answers an error, and rejects when the
	// request itself fails, so an empty table means the request got nowhere. A
	// swap takes the note with it, and an answer that never came leaves it here.
	areaResults.innerHTML =
		'<div class="loadingNote"><span>area stats loading</span></div>';
	const done = () => {
		areaResults.querySelector(".loadingNote")?.remove();
		setAreaLoaded(areaResults.innerHTML != "");
	};
	htmx.ajax("GET", "/areastats?" + p.toString(), "#areaStatsResults").then(done, done);
});
