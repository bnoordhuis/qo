"use strict"

go()

async function go() {
	console.log("before")
	const r = await get("https://github.com/quickjs-ng/quickjs")
	//const r = await get("https://google.com/")
	for (;;) {
		const b = await read(r)
		if (!b)
			break
		console.log(b)
	}
	close(r)
}
