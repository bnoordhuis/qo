"use strict"

go()

async function go() {
	try {
		console.log("before")
		const r = await get("")
		//const r = await get("https://github.com/quickjs-ng/quickjs")
		console.log("after", r)
	} catch (e) {
		console.log("fatal error:", e)
	}
}
