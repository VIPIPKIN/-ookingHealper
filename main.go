package main

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"

	_ "github.com/mattn/go-sqlite3"
)

type Ingredient struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
	Price    float64 `json:"price"`
}

type Recipe struct {
	ID          int          `json:"id"`
	Name        string       `json:"name"`
	Ingredients []Ingredient `json:"ingredients"`
	TotalCost   float64      `json:"total_cost"`
}

var db *sql.DB

func main() {
	// Инициализация базы данных
	initDB()
	defer db.Close()

	// Настройка маршрутов
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/save-recipe", saveRecipeHandler)
	http.HandleFunc("/api/recipes", getRecipesHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Println("Сервер запущен на http://localhost:8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("Ошибка запуска сервера:", err)
	}
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./recipes.db")
	if err != nil {
		log.Fatal("Ошибка подключения к БД:", err)
	}

	// Создаём таблицы
	createTableSQL := `
    CREATE TABLE IF NOT EXISTS recipes (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT,
        total_cost REAL,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );

    CREATE TABLE IF NOT EXISTS ingredients (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        recipe_id INTEGER,
        name TEXT,
        quantity REAL,
        unit TEXT,
        price REAL,
        FOREIGN KEY (recipe_id) REFERENCES recipes(id)
    );`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatal("Ошибка создания таблиц:", err)
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, "Ошибка загрузки шаблона: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

func saveRecipeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не разрешен", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Ошибка обработки формы", http.StatusBadRequest)
		return
	}

	// Получаем данные из формы
	names := r.Form["name[]"]
	boughtQuantities := r.Form["bought_quantity[]"]
	units := r.Form["unit[]"]
	prices := r.Form["price[]"]
	usedQuantities := r.Form["used_quantity[]"]

	// Считаем общую стоимость
	var total float64
	var ingredients []Ingredient

	for i := 0; i < len(names); i++ {
		if names[i] == "" {
			continue
		}

		// Парсим числа
		boughtQty, _ := strconv.ParseFloat(boughtQuantities[i], 64)
		price, _ := strconv.ParseFloat(prices[i], 64)
		usedQty, _ := strconv.ParseFloat(usedQuantities[i], 64)

		// Расчёт: (цена / купили) * использовали
		var cost float64
		if boughtQty > 0 {
			cost = (price / boughtQty) * usedQty
		}
		total += cost

		ingredients = append(ingredients, Ingredient{
			Name:     names[i],
			Quantity: usedQty,
			Unit:     units[i],
			Price:    price,
		})
	}

	// Сохраняем в базу данных
	result, err := db.Exec("INSERT INTO recipes (name, total_cost) VALUES (?, ?)",
		"Рецепт", total)
	if err != nil {
		http.Error(w, "Ошибка сохранения рецепта: "+err.Error(), http.StatusInternalServerError)
		return
	}

	recipeID, _ := result.LastInsertId()

	// Сохраняем ингредиенты
	for _, ing := range ingredients {
		_, err = db.Exec(`
            INSERT INTO ingredients (recipe_id, name, quantity, unit, price) 
            VALUES (?, ?, ?, ?, ?)`,
			recipeID, ing.Name, ing.Quantity, ing.Unit, ing.Price)
		if err != nil {
			log.Printf("Ошибка сохранения ингредиента: %v", err)
		}
	}

	// Возвращаем успешный ответ
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"total":   total,
		"id":      recipeID,
	})
}

func getRecipesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
        SELECT id, name, total_cost, created_at 
        FROM recipes 
        ORDER BY created_at DESC 
        LIMIT 10`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var recipes []Recipe
	for rows.Next() {
		var r Recipe
		var createdAt string
		err := rows.Scan(&r.ID, &r.Name, &r.TotalCost, &createdAt)
		if err != nil {
			continue
		}
		recipes = append(recipes, r)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recipes)
}
