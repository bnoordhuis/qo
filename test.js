"use strict"

go()

async function go() {
	const r = await request("GET", "https://github.com/quickjs-ng/quickjs")
	for (;;) {
		const b = await read(r)
		if (!b)
			break
		console.log(b)
	}
	close(r)
}
