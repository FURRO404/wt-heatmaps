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
	document
		.getElementById("heat")
		.setAttribute("href", "/heat?" + new URLSearchParams(f).toString());
	document
		.getElementById("tankmap")
		.setAttribute("href", "/minimap/2048/" + f.get("level"));
});

var applySettingsLevel = (e) => {
	console.log(e);
};

// area selection, drags a box and asks the server for the vehicles in it

const mapSize = 2048;
const selectBtn = document.getElementById("areaSelectBtn");
const selectRect = document.getElementById("areaSelect");
let selectMode = false;
let selectFrom = null;

selectBtn.addEventListener("click", () => {
	setSelectMode(!selectMode);
});

function setSelectMode(on) {
	selectMode = on;
	svg.style.cursor = on ? "crosshair" : "";
	selectBtn.style.fontWeight = on ? "bold" : "";
}

function svgPoint(e) {
	const rect = svg.getBoundingClientRect();
	return {
		x: vb.x + ((e.clientX - rect.left) / rect.width) * vb.w,
		y: vb.y + ((e.clientY - rect.top) / rect.height) * vb.h,
	};
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
	if (from.x == to.x || from.y == to.y) {
		selectRect.style.display = "none";
		return;
	}
	const f = new FormData(form);
	if (f.get("level") == "") {
		return;
	}
	const p = new URLSearchParams(f);
	p.set("u0", from.x / mapSize);
	p.set("v0", from.y / mapSize);
	p.set("u1", to.x / mapSize);
	p.set("v1", to.y / mapSize);
	htmx.ajax("GET", "/areastats?" + p.toString(), "#areaStats");
});
